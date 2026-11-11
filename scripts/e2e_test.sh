#!/bin/bash
#
# End-to-end smoke test: brings up the real Docker stack (Order, Inventory,
# Payment, Assembly, Order/Inventory's Postgres instances and Kafka, via the
# single deploy/compose/docker-compose.yaml), applies migrations, and drives
# the Order HTTP API with curl — including the async Kafka chain that takes a
# paid order to ASSEMBLED.
#
# Usage: scripts/e2e_test.sh
# (also wired up as `task test:e2e`)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="http://localhost:8080"

FAILURES=0
STACK_UP=0

# Seed data from inventory/migrations/00001_create_parts_table.sql — keep in
# sync if it changes.
HULL_ALUMINUM=550e8400-e29b-41d4-a716-446655440001
HULL_TITANIUM=550e8400-e29b-41d4-a716-446655440002
ENGINE_ION_C=550e8400-e29b-41d4-a716-446655440003
ENGINE_ION_B=550e8400-e29b-41d4-a716-446655440004
SHIELD_ENERGY=550e8400-e29b-41d4-a716-446655440005
HULL_OUT_OF_STOCK=550e8400-e29b-41d4-a716-446655440007
RANDOM_UUID=00000000-0000-4000-8000-000000000000
USER_UUID=7c9e6679-7425-40de-944b-e07fc1f90ae7

cleanup() {
	echo
	echo "==> Stopping the infrastructure..."
	if [ "$STACK_UP" -eq 1 ]; then
		task -d "$REPO_ROOT" down >/dev/null 2>&1
	fi
}
trap cleanup EXIT

wait_for_port() {
	local port="$1" name="$2"
	for _ in $(seq 1 100); do
		if curl -s -o /dev/null "http://localhost:$port" 2>/dev/null; then
			return 0
		fi
		if nc -z localhost "$port" 2>/dev/null; then
			return 0
		fi
		sleep 0.2
	done
	echo "ERROR: $name did not start listening on :$port in time" >&2
	return 1
}

# assert_status METHOD PATH BODY EXPECTED_STATUS [EXPECTED_BODY_SUBSTRING]
assert_status() {
	local method="$1" path="$2" body="$3" expected="$4" want_substr="${5:-}"
	local resp status actual_body

	if [ -n "$body" ]; then
		resp=$(curl -s -w '\n%{http_code}' -X "$method" "$BASE_URL$path" -H 'Content-Type: application/json' -d "$body")
	else
		resp=$(curl -s -w '\n%{http_code}' -X "$method" "$BASE_URL$path")
	fi

	status=$(echo "$resp" | tail -n1)
	actual_body=$(echo "$resp" | sed '$d')

	if [ "$status" != "$expected" ]; then
		echo "FAIL: $method $path -> expected HTTP $expected, got $status (body: $actual_body)"
		FAILURES=$((FAILURES + 1))
		return 1
	fi

	if [ -n "$want_substr" ] && [[ "$actual_body" != *"$want_substr"* ]]; then
		echo "FAIL: $method $path -> expected body to contain '$want_substr', got: $actual_body"
		FAILURES=$((FAILURES + 1))
		return 1
	fi

	echo "PASS: $method $path -> $status"
	LAST_BODY="$actual_body"
}

echo "==> Starting the infrastructure (Order, Inventory, Payment, Assembly, Postgres, Kafka)..."
task -d "$REPO_ROOT" up || exit 1
STACK_UP=1

wait_for_port 50051 InventoryService || exit 1
wait_for_port 50052 PaymentService || exit 1
wait_for_port 8080 OrderService || exit 1

echo "==> Applying migrations..."
task -d "$REPO_ROOT" migrate:inventory:up || exit 1
task -d "$REPO_ROOT" migrate:order:up || exit 1

echo
echo "==> Running e2e scenarios..."

# 1-4: create -> get (pending) -> pay -> get (paid)
assert_status POST /api/v1/orders "{\"user_uuid\":\"$USER_UUID\",\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 201 '"total_price":800000'
ORDER1=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"PENDING_PAYMENT"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 200
assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"PAID"'

# 5-7: create -> cancel -> get (cancelled)
assert_status POST /api/v1/orders "{\"user_uuid\":\"$USER_UUID\",\"hull_uuid\":\"$HULL_TITANIUM\",\"engine_uuid\":\"$ENGINE_ION_B\",\"shield_uuid\":\"$SHIELD_ENERGY\"}" 201 '"total_price":2700000'
ORDER2=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status POST "/api/v1/orders/$ORDER2/cancel" "" 200
assert_status GET "/api/v1/orders/$ORDER2" "" 200 '"status":"CANCELLED"'

# 8-11: not found / conflict cases. ORDER1 may already be ASSEMBLED by now
# (the Kafka chain runs in the background), so only its status code is pinned.
assert_status GET "/api/v1/orders/$RANDOM_UUID" "" 404 '"message":"order not found"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 409
assert_status POST "/api/v1/orders/$ORDER2/pay" '{"payment_method":"CARD"}' 409 'order is cancelled'
assert_status POST "/api/v1/orders/$ORDER1/cancel" "" 409

# 12-13: inventory-driven errors
assert_status POST /api/v1/orders "{\"user_uuid\":\"$USER_UUID\",\"hull_uuid\":\"$RANDOM_UUID\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 404 'component not found'
assert_status POST /api/v1/orders "{\"user_uuid\":\"$USER_UUID\",\"hull_uuid\":\"$HULL_OUT_OF_STOCK\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 409 'component out of stock'

# 14-16: request validation (ogen)
assert_status POST /api/v1/orders '{}' 400
assert_status GET /api/v1/orders/not-a-uuid "" 400
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"BITCOIN"}' 400

# 17: missing user_uuid is rejected
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 400

# 18: Kafka chain — OrderPaid -> AssemblyService -> ShipAssembled -> ASSEMBLED
# (assembly takes 5-15s, so allow up to 45s)
assembled=0
for _ in $(seq 1 90); do
	if curl -s "$BASE_URL/api/v1/orders/$ORDER1" | grep -q '"status":"ASSEMBLED"'; then
		assembled=1
		break
	fi
	sleep 0.5
done
if [ "$assembled" -eq 1 ]; then
	echo "PASS: ORDER1 reached ASSEMBLED via Kafka"
else
	echo "FAIL: ORDER1 never reached ASSEMBLED (last: $(curl -s "$BASE_URL/api/v1/orders/$ORDER1"))"
	FAILURES=$((FAILURES + 1))
fi
assert_status POST "/api/v1/orders/$ORDER1/cancel" "" 409 'order is already assembled'

# 19: CommitParts wrote ORDER1's parts off — stock down by one, nothing left reserved
for part in "$HULL_ALUMINUM" "$ENGINE_ION_C"; do
	row=$(docker exec ms-ex-inventory-postgres-1 psql -U inventory -d inventory -tA \
		-c "SELECT stock_quantity || ',' || reserved FROM parts WHERE uuid = '$part'")
	if [ "$row" = "99999,0" ]; then
		echo "PASS: part $part committed (stock,reserved = $row)"
	else
		echo "FAIL: part $part expected stock,reserved = 99999,0, got $row"
		FAILURES=$((FAILURES + 1))
	fi
done

# 20: Kafka UI is up (a JVM app — give it time to finish starting)
ui_status=000
for _ in $(seq 1 60); do
	ui_status=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8090)
	[ "$ui_status" = "200" ] && break
	sleep 1
done
if [ "$ui_status" = "200" ]; then
	echo "PASS: Kafka UI on localhost:8090 -> 200"
else
	echo "FAIL: Kafka UI on localhost:8090 -> $ui_status"
	FAILURES=$((FAILURES + 1))
fi

# 21: data survives a container restart (real Postgres persistence)
docker restart ms-ex-order-1 >/dev/null 2>&1
wait_for_port 8080 OrderService || exit 1
assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"ASSEMBLED"'

echo
if [ "$FAILURES" -eq 0 ]; then
	echo "==> All e2e scenarios passed."
	exit 0
else
	echo "==> $FAILURES scenario(s) failed."
	echo "--- infrastructure logs ---"
	docker compose -f "$REPO_ROOT/deploy/compose/docker-compose.yaml" logs 2>&1
	exit 1
fi
