#!/bin/bash
#
# End-to-end smoke test: builds all three services, runs them as real
# standalone processes (real TCP sockets, no bufconn/httptest), drives the
# Order HTTP API with curl, and asserts on status codes and response bodies.
#
# Usage: scripts/e2e_test.sh
# (also wired up as `task test:e2e`)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d /tmp/ms-ex-e2e.XXXXXX)"
BASE_URL="http://localhost:8080"

INVENTORY_PID=""
PAYMENT_PID=""
ORDER_PID=""
FAILURES=0

# Seed data from inventory/pkg/repository/memory.go — keep in sync if it changes.
HULL_ALUMINUM=550e8400-e29b-41d4-a716-446655440001
HULL_TITANIUM=550e8400-e29b-41d4-a716-446655440002
ENGINE_ION_C=550e8400-e29b-41d4-a716-446655440003
ENGINE_ION_B=550e8400-e29b-41d4-a716-446655440004
SHIELD_ENERGY=550e8400-e29b-41d4-a716-446655440005
HULL_OUT_OF_STOCK=550e8400-e29b-41d4-a716-446655440007
RANDOM_UUID=00000000-0000-4000-8000-000000000000

cleanup() {
	echo
	echo "==> Stopping services..."
	for pid in "$ORDER_PID" "$PAYMENT_PID" "$INVENTORY_PID"; do
		if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
			kill "$pid" 2>/dev/null
		fi
	done
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

wait_for_port() {
	local port="$1" name="$2"
	for _ in $(seq 1 50); do
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

echo "==> Building services..."
go -C "$REPO_ROOT" build -o "$WORK_DIR/inventory" ./inventory/cmd || exit 1
go -C "$REPO_ROOT" build -o "$WORK_DIR/payment" ./payment/cmd || exit 1
go -C "$REPO_ROOT" build -o "$WORK_DIR/order" ./order/cmd || exit 1

echo "==> Starting InventoryService (:50051)..."
"$WORK_DIR/inventory" >"$WORK_DIR/inventory.log" 2>&1 &
INVENTORY_PID=$!

echo "==> Starting PaymentService (:50052)..."
"$WORK_DIR/payment" >"$WORK_DIR/payment.log" 2>&1 &
PAYMENT_PID=$!

wait_for_port 50051 InventoryService || exit 1
wait_for_port 50052 PaymentService || exit 1

echo "==> Starting OrderService (:8080)..."
"$WORK_DIR/order" >"$WORK_DIR/order.log" 2>&1 &
ORDER_PID=$!

wait_for_port 8080 OrderService || exit 1

echo
echo "==> Running e2e scenarios..."

# 1-4: create -> get (pending) -> pay -> get (paid)
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 201 '"total_price":800000'
ORDER1=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"PENDING_PAYMENT"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 200
assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"PAID"'

# 5-7: create -> cancel -> get (cancelled)
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_TITANIUM\",\"engine_uuid\":\"$ENGINE_ION_B\",\"shield_uuid\":\"$SHIELD_ENERGY\"}" 201 '"total_price":2700000'
ORDER2=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status POST "/api/v1/orders/$ORDER2/cancel" "" 200
assert_status GET "/api/v1/orders/$ORDER2" "" 200 '"status":"CANCELLED"'

# 8-11: not found / conflict cases
assert_status GET "/api/v1/orders/$RANDOM_UUID" "" 404 '"message":"order not found"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 409 'already paid or cancelled'
assert_status POST "/api/v1/orders/$ORDER2/pay" '{"payment_method":"CARD"}' 409 'already paid or cancelled'
assert_status POST "/api/v1/orders/$ORDER1/cancel" "" 409 'already paid or cancelled'

# 12-13: inventory-driven errors
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$RANDOM_UUID\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 404 'component not found'
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_OUT_OF_STOCK\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 409 'component out of stock'

# 14-16: request validation (ogen)
assert_status POST /api/v1/orders '{}' 400
assert_status GET /api/v1/orders/not-a-uuid "" 400
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"BITCOIN"}' 400

echo
if [ "$FAILURES" -eq 0 ]; then
	echo "==> All e2e scenarios passed."
	exit 0
else
	echo "==> $FAILURES scenario(s) failed."
	echo "--- inventory.log ---"; cat "$WORK_DIR/inventory.log"
	echo "--- payment.log ---"; cat "$WORK_DIR/payment.log"
	echo "--- order.log ---"; cat "$WORK_DIR/order.log"
	exit 1
fi
