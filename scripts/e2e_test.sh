#!/bin/bash
#
# End-to-end smoke test: brings up the real Docker stack with observability
# (`task up-all`: Order, Inventory, Payment, Assembly, IAM, their Postgres
# instances, Redis, Kafka, and the OTel Collector with Elasticsearch/Kibana,
# Prometheus/Grafana and Jaeger), applies migrations, registers and logs in a
# user through IAM (grpcurl), and drives the Order HTTP API with curl as that
# user — including the async Kafka chain that takes a paid order to ASSEMBLED.
# Finally it checks that the run's logs, business metrics and distributed
# traces arrived in Elasticsearch, Prometheus and Jaeger.
#
# Usage: scripts/e2e_test.sh
# (also wired up as `task test:e2e`)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="http://localhost:8080"
COMPOSE_NETWORK="ms-ex_default"
GRPCURL_IMAGE="fullstorydev/grpcurl:v1.9.3"

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

LOGIN="e2e-user-$(date +%s)"
PASSWORD="e2e-password"

# Authorization header value sent by assert_status/curl_auth; set after login.
# Prefix a call with `AUTH=` (or a bogus value) to send no / another token.
AUTH=""

cleanup() {
	echo
	echo "==> Stopping the infrastructure..."
	if [ "$STACK_UP" -eq 1 ]; then
		task -d "$REPO_ROOT" down >/dev/null 2>&1
	fi
}
trap cleanup EXIT

fail() {
	echo "FAIL: $*"
	FAILURES=$((FAILURES + 1))
}

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

# grpc ADDRESS METHOD JSON [extra grpcurl args...] — calls a service on the
# compose network via a throwaway grpcurl container (uses server reflection).
# Prints the response (or grpcurl's error, which includes the status code).
grpc() {
	local addr="$1" method="$2" data="$3"
	shift 3
	docker run --rm --network "$COMPOSE_NETWORK" "$GRPCURL_IMAGE" -plaintext "$@" -d "$data" "$addr" "$method" 2>&1
}

# curl_auth ARGS... — curl with the current AUTH header (if any).
curl_auth() {
	if [ -n "$AUTH" ]; then
		curl -s -H "Authorization: $AUTH" "$@"
	else
		curl -s "$@"
	fi
}

# assert_status METHOD PATH BODY EXPECTED_STATUS [EXPECTED_BODY_SUBSTRING]
assert_status() {
	local method="$1" path="$2" body="$3" expected="$4" want_substr="${5:-}"
	local resp status actual_body

	if [ -n "$body" ]; then
		resp=$(curl_auth -w '\n%{http_code}' -X "$method" "$BASE_URL$path" -H 'Content-Type: application/json' -d "$body")
	else
		resp=$(curl_auth -w '\n%{http_code}' -X "$method" "$BASE_URL$path")
	fi

	status=$(echo "$resp" | tail -n1)
	actual_body=$(echo "$resp" | sed '$d')

	if [ "$status" != "$expected" ]; then
		fail "$method $path -> expected HTTP $expected, got $status (body: $actual_body)"
		return 1
	fi

	if [ -n "$want_substr" ] && [[ "$actual_body" != *"$want_substr"* ]]; then
		fail "$method $path -> expected body to contain '$want_substr', got: $actual_body"
		return 1
	fi

	echo "PASS: $method $path -> $status"
	LAST_BODY="$actual_body"
}

# poll DESCRIPTION TIMEOUT_SECONDS COMMAND... — PASS once COMMAND succeeds,
# FAIL if it never does within the timeout (telemetry arrives asynchronously).
poll() {
	local desc="$1" timeout="$2"
	shift 2
	for _ in $(seq 1 "$timeout"); do
		if "$@" >/dev/null 2>&1; then
			echo "PASS: $desc"
			return 0
		fi
		sleep 1
	done
	fail "$desc (not within ${timeout}s)"
}

# assert_contains DESCRIPTION HAYSTACK NEEDLE
assert_contains() {
	if [[ "$2" == *"$3"* ]]; then
		echo "PASS: $1"
	else
		fail "$1 -> expected '$3' in: $2"
	fi
}

echo "==> Starting the infrastructure with observability (task up-all)..."
task -d "$REPO_ROOT" up-all || exit 1
STACK_UP=1

wait_for_port 50053 IAMService || exit 1
wait_for_port 50051 InventoryService || exit 1
wait_for_port 50052 PaymentService || exit 1
wait_for_port 8080 OrderService || exit 1

echo "==> Applying migrations..."
task -d "$REPO_ROOT" migrate:inventory:up || exit 1
task -d "$REPO_ROOT" migrate:order:up || exit 1
task -d "$REPO_ROOT" migrate:iam:up || exit 1

echo
echo "==> Running e2e scenarios..."

# A1-A2: register and log in through IAM (gRPC)
resp=$(grpc iam:50053 auth.v1.AuthService/Register "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}")
USER_UUID=$(echo "$resp" | jq -r '.userUuid // empty' 2>/dev/null)
if [ -n "$USER_UUID" ]; then echo "PASS: IAM Register -> user $USER_UUID"; else fail "IAM Register: $resp"; exit 1; fi

resp=$(grpc iam:50053 auth.v1.AuthService/Login "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}")
SESSION=$(echo "$resp" | jq -r '.sessionUuid // empty' 2>/dev/null)
if [ -n "$SESSION" ]; then echo "PASS: IAM Login -> session issued"; else fail "IAM Login: $resp"; exit 1; fi
AUTH="Bearer $SESSION"

# A3-A6: IAM error cases, Whoami and GetUser. (Request bodies go through
# variables: a JSON object with a comma inside "$(...)" would be brace-expanded.)
duplicate_body="{\"login\":\"$LOGIN\",\"password\":\"other\"}"
wrong_password_body="{\"login\":\"$LOGIN\",\"password\":\"wrong\"}"
assert_contains "IAM Register duplicate login -> AlreadyExists" \
	"$(grpc iam:50053 auth.v1.AuthService/Register "$duplicate_body")" "AlreadyExists"
assert_contains "IAM Login wrong password -> Unauthenticated" \
	"$(grpc iam:50053 auth.v1.AuthService/Login "$wrong_password_body")" "Unauthenticated"
assert_contains "IAM Whoami -> session's user" \
	"$(grpc iam:50053 auth.v1.AuthService/Whoami "{\"session_uuid\":\"$SESSION\"}")" "$USER_UUID"
assert_contains "IAM GetUser -> login" \
	"$(grpc iam:50053 user.v1.UserService/GetUser "{\"user_uuid\":\"$USER_UUID\"}")" "$LOGIN"

# A7: the session lives in Redis as a hash with a TTL of at most 24h
ttl=$(docker exec ms-ex-redis-1 redis-cli TTL "session:$SESSION" | tr -d '\r')
stored_user=$(docker exec ms-ex-redis-1 redis-cli HGET "session:$SESSION" user_uuid | tr -d '\r')
if [ "$ttl" -gt 0 ] 2>/dev/null && [ "$ttl" -le 86400 ] && [ "$stored_user" = "$USER_UUID" ]; then
	echo "PASS: Redis session:{uuid} hash holds the user, TTL ${ttl}s"
else
	fail "Redis session -> TTL '$ttl', user_uuid '$stored_user'"
fi

# A8-A9: Inventory authenticates from incoming gRPC metadata
assert_contains "Inventory ListParts without session -> Unauthenticated" \
	"$(grpc inventory:50051 inventory.v1.InventoryService/ListParts '{}')" "Unauthenticated"
assert_contains "Inventory ListParts with session-uuid metadata -> parts" \
	"$(grpc inventory:50051 inventory.v1.InventoryService/ListParts '{}' -H "session-uuid: $SESSION")" "Aluminum Hull"

# A10-A11: Order rejects requests without a valid session (401)
AUTH="" assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 401 '"code":401'
AUTH="Bearer $RANDOM_UUID" assert_status GET "/api/v1/orders/$RANDOM_UUID" "" 401 '"code":401'

# 1-4: create -> get (pending, owned by the session's user) -> pay -> get (paid)
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 201 '"total_price":800000'
ORDER1=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status GET "/api/v1/orders/$ORDER1" "" 200 "\"user_uuid\":\"$USER_UUID\""
assert_contains "ORDER1 is PENDING_PAYMENT" "$LAST_BODY" '"status":"PENDING_PAYMENT"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 200
assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"PAID"'

# 5-7: create -> cancel -> get (cancelled)
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_TITANIUM\",\"engine_uuid\":\"$ENGINE_ION_B\",\"shield_uuid\":\"$SHIELD_ENERGY\"}" 201 '"total_price":2700000'
ORDER2=$(echo "$LAST_BODY" | jq -r .order_uuid)

assert_status POST "/api/v1/orders/$ORDER2/cancel" "" 200
assert_status GET "/api/v1/orders/$ORDER2" "" 200 '"status":"CANCELLED"'

# 8-11: not found / conflict cases. ORDER1 may already be ASSEMBLED by now
# (the Kafka chain runs in the background), so only its status code is pinned.
assert_status GET "/api/v1/orders/$RANDOM_UUID" "" 404 '"message":"order not found"'
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"CARD"}' 409
assert_status POST "/api/v1/orders/$ORDER2/pay" '{"payment_method":"CARD"}' 409 'order is cancelled'
assert_status POST "/api/v1/orders/$ORDER1/cancel" "" 409

# 12-13: inventory-driven errors (Order forwards the session to Inventory)
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$RANDOM_UUID\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 404 'component not found'
assert_status POST /api/v1/orders "{\"hull_uuid\":\"$HULL_OUT_OF_STOCK\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 409 'component out of stock'

# 14-16: request validation (ogen)
assert_status POST /api/v1/orders '{}' 400
assert_status GET /api/v1/orders/not-a-uuid "" 400
assert_status POST "/api/v1/orders/$ORDER1/pay" '{"payment_method":"BITCOIN"}' 400

# 17: a user_uuid in the body is ignored — the order belongs to the session's user
assert_status POST /api/v1/orders "{\"user_uuid\":\"$RANDOM_UUID\",\"hull_uuid\":\"$HULL_ALUMINUM\",\"engine_uuid\":\"$ENGINE_ION_C\"}" 201
ORDER3=$(echo "$LAST_BODY" | jq -r .order_uuid)
assert_status GET "/api/v1/orders/$ORDER3" "" 200 "\"user_uuid\":\"$USER_UUID\""

# 18: Kafka chain — OrderPaid -> AssemblyService -> ShipAssembled -> ASSEMBLED.
# The ShipAssembled consumer has no user session; CommitParts is exempt from auth.
# (assembly takes 5-15s, so allow up to 45s)
assembled=0
for _ in $(seq 1 90); do
	if curl_auth "$BASE_URL/api/v1/orders/$ORDER1" | grep -q '"status":"ASSEMBLED"'; then
		assembled=1
		break
	fi
	sleep 0.5
done
if [ "$assembled" -eq 1 ]; then
	echo "PASS: ORDER1 reached ASSEMBLED via Kafka"
else
	fail "ORDER1 never reached ASSEMBLED (last: $(curl_auth "$BASE_URL/api/v1/orders/$ORDER1"))"
fi
assert_status POST "/api/v1/orders/$ORDER1/cancel" "" 409 'order is already assembled'

# 19: CommitParts wrote ORDER1's parts off. ORDER3 (same parts) is still
# reserved, so stock is down by one and one unit stays reserved.
for part in "$HULL_ALUMINUM" "$ENGINE_ION_C"; do
	row=$(docker exec ms-ex-inventory-postgres-1 psql -U inventory -d inventory -tA \
		-c "SELECT stock_quantity || ',' || reserved FROM parts WHERE uuid = '$part'")
	if [ "$row" = "99999,1" ]; then
		echo "PASS: part $part committed (stock,reserved = $row)"
	else
		fail "part $part expected stock,reserved = 99999,1, got $row"
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
	fail "Kafka UI on localhost:8090 -> $ui_status"
fi

# 21-30: observability (asserted here, after the flows above produced telemetry)

# UIs answer
poll "Kibana on localhost:5601 is available" 180 sh -c "curl -fsS http://localhost:5601/api/status | grep -q '\"level\":\"available\"'"
poll "Kibana data view otel-logs exists" 60 curl -fsS http://localhost:5601/api/data_views/data_view/otel-logs
poll "Grafana on localhost:3000 is healthy" 60 sh -c "curl -fsS http://localhost:3000/api/health | grep -q '\"database\": *\"ok\"'"
poll "Grafana provisioned the Orders dashboard" 30 curl -fsS http://localhost:3000/api/dashboards/uid/orders
poll "Jaeger UI on localhost:16686 answers" 30 curl -fsS -o /dev/null http://localhost:16686/

# Logs: every service's logs reach the otel-logs data stream
logs_from() {
	curl -fsS "http://localhost:9200/otel-logs/_count" -H 'Content-Type: application/json' \
		-d "{\"query\":{\"term\":{\"resource.attributes.service.name\":\"$1\"}}}" | jq -e '.count > 0'
}
for svc in order-service inventory-service payment-service iam-service assembly-service; do
	poll "logs of $svc in Elasticsearch otel-logs" 60 logs_from "$svc"
done

# Metrics: business metrics reach Prometheus (pushed every 10s)
prom_at_least() {
	curl -fsS -G http://localhost:9090/api/v1/query --data-urlencode "query=sum($1)" |
		jq -e --argjson min "$2" '(.data.result[0].value[1] // "0" | tonumber) >= $min'
}
poll "Prometheus orders_created_total >= 3" 60 prom_at_least orders_created_total 3
poll "Prometheus orders_paid_total{payment_method=CARD} >= 1" 60 prom_at_least 'orders_paid_total{payment_method="CARD"}' 1
poll "Prometheus orders_cancelled_total >= 1" 60 prom_at_least orders_cancelled_total 1
poll "Prometheus orders_assembled_total >= 1" 60 prom_at_least orders_assembled_total 1

# Traces: one create-order trace spans Order -> IAM and Order -> Inventory -> IAM;
# the pay trace reaches Payment
trace_spans() {
	local op="$1"
	shift
	curl -fsS -G "http://localhost:16686/api/traces" --data-urlencode service=order-service \
		--data-urlencode "operation=$op" --data-urlencode limit=20 |
		jq -e --args '[.data[] | [.processes[].serviceName] | unique] | any(. as $s | ($ARGS.positional | all(. as $want | $s | index($want))))' "$@"
}
poll "Jaeger trace of POST /api/v1/orders spans order, iam, inventory" 60 \
	trace_spans "POST /api/v1/orders" order-service iam-service inventory-service
poll "Jaeger trace of POST /api/v1/orders/{order_uuid}/pay spans order, iam, payment" 60 \
	trace_spans "POST /api/v1/orders/{order_uuid}/pay" order-service iam-service payment-service

# 31: data survives an Order restart (Postgres), and so does the session (Redis)
docker restart ms-ex-order-1 >/dev/null 2>&1
wait_for_port 8080 OrderService || exit 1
assert_status GET "/api/v1/orders/$ORDER1" "" 200 '"status":"ASSEMBLED"'

# 32: logout deletes the session — the same token is rejected afterwards
grpc iam:50053 auth.v1.AuthService/Logout "{\"session_uuid\":\"$SESSION\"}" >/dev/null
exists=$(docker exec ms-ex-redis-1 redis-cli EXISTS "session:$SESSION" | tr -d '\r')
if [ "$exists" = "0" ]; then echo "PASS: Logout removed session:{uuid} from Redis"; else fail "session still in Redis after Logout"; fi
assert_status GET "/api/v1/orders/$ORDER1" "" 401 '"code":401'

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
