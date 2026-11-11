---
name: add-grpc-method
description: Add a new RPC to InventoryService or PaymentService end to end — proto, generation, server handler, service logic, client interface in the caller, mocks and tests. Use when a task says "add gRPC method X" or a service needs a new call to another service.
---

# Add a gRPC method

Example in history: `CommitParts` in Inventory (week 5).

1. **Contract** — `shared/proto/<svc>/v1/<svc>.proto`:
   - `rpc X(XRequest) returns (XResponse);` with a comment on the rpc.
   - New `XRequest` / `XResponse` messages (even if empty), every message and field commented.
   - `task proto:lint && task proto:gen`.
2. **Domain** (if it changes state, Inventory) — a method on the aggregate in
   `pkg/domain` that guards invariants and returns sentinel errors; unit tests in `*_test.go`.
3. **Service** — `pkg/service`:
   - Method on `Service`. Read-modify-write goes through `trManager.Do` +
     `…ForUpdate` reads; multiple rows → sorted keys (see `.claude/rules/persistence.md`).
   - New repository methods → add to the service's `Repository` interface and both
     repository implementations (Postgres + in-memory).
4. **Transport** — `pkg/api/grpc.go`:
   - Add the method to the api's `Service` interface and a `Server.X` handler
     (validate/convert, call service, `return nil, toStatusErr(err)`).
   - Map any new sentinel error in the error switch.
5. **Caller** (usually Order) — add the method to the consumer-owned client
   interface (`order/pkg/service.InventoryClient`), use it in the service.
6. `task mocks:gen`.
7. **Tests**: domain, service (success, each error, locking order if relevant),
   gRPC handler (status code per error), caller service (mocked client).
   If the behavior is visible through the Order API, add an API test in
   `order/tests` and an e2e scenario.
8. Run the `verify` skill. Update `docs/architecture.md` if the flow changed.
