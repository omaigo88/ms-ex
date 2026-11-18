---
paths:
  - "**/*_test.go"
  - "order/tests/**"
  - "scripts/e2e_test.sh"
  - ".mockery.yaml"
---

# Testing

Three levels; a change usually touches the first two, sometimes the third.

1. **Unit** (`task test`, `-race`) — external test packages (`package service_test`),
   testify `require`/`assert`, mockery mocks from `<pkg>/mocks`.
   - Use the typed expecter API: `m.EXPECT().Method(args).Return(...)`;
     `RunAndReturn` to inspect arguments. Mocks are created with `mocks.NewMockX(t)`
     so unmet expectations fail the test.
   - Never write mocks by hand. New interface → it's picked up by
     `.mockery.yaml` (`all: true` per package) → `task mocks:gen`. A new package
     with interfaces needs an entry in `.mockery.yaml`.
   - Table-driven tests for status/error matrices (see
     `TestService_CancelOrder_RejectsNonPending`). Tests build deps through a
     small helper (`newDeps(t)`), not copy-pasted setup.
   - Cover: happy path, each sentinel error, idempotent redelivery for event handlers,
     "malformed message is dropped" for Kafka handlers.
2. **API** (`task test:api`, `order/tests`) — real Order HTTP server, real
   IAM/Inventory/Payment gRPC servers over in-memory repositories, no-op
   `TrManager` and no-op publisher. No Docker, no Kafka, no Redis. Auth is
   real: `TestMain` registers and logs in a user; `httpClient` sends its
   Bearer token, `anonClient` sends none; `inventoryClient` calls Inventory
   with the session in metadata. The async chain is exercised by calling
   `orderSvc.MarkAssembled` directly (no session, like the Kafka consumer).
3. **E2E** (`task test:e2e`, `scripts/e2e_test.sh`) — full Docker stack, real
   Postgres + Redis + Kafka, curl + jq + psql + grpcurl (run as a container
   on the compose network — no host install). Brings the stack up and tears it down.
   - JSON bodies with commas go into variables before `"$(...)"` — inline they
     get brace-expanded.
   - Async assertions poll with a timeout; never `sleep N` and hope.
   - Assert on status code always; on body substrings only when the message is a
     stable sentinel.
   - Number new scenarios and keep the header comment accurate.

Coverage threshold is 40% (`task test:coverage`); generated code and mocks are excluded.
