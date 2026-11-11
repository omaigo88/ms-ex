---
name: change-order-api
description: Change the public Order HTTP API — add/modify endpoints, request/response fields, enum values (e.g. order statuses) or error responses — via the OpenAPI spec and ogen, then handler, service, tests and e2e. Use for any change visible to HTTP clients of OrderService.
---

# Change the Order HTTP API

1. **Spec** — edit the right file under `shared/api/order/v1/`
   (`paths/`, `components/*.yaml`, `components/enums/`, `components/responses/`).
   New service-relevant fields: `required` + `format: uuid` / enum `$ref`.
2. `task ogen:gen` — read the generated method/type names in
   `shared/pkg/openapi/order/v1` (don't edit them).
3. **Handler** `order/pkg/api/handler.go`: map ogen types ⇄ service inputs /
   `repository.Order` (`toOrderDto`), map errors (`isOrderStatusConflict` etc.)
   to the generated response types. A new endpoint = a new method on the
   ogen `Handler` interface.
4. **Service / repository** as needed (new status → constant in
   `order/pkg/repository`, transitions in `order/pkg/service`, `statusErr`).
5. `task mocks:gen` if interfaces changed.
6. **Tests**:
   - `order/pkg/api/handler_test.go` — status code per outcome, field passthrough;
   - `order/pkg/service/service_test.go` — business rules (table tests for status matrices);
   - `order/tests/api_test.go` — real HTTP round trip (update request/DTO structs there);
   - `scripts/e2e_test.sh` — add a numbered scenario; include new required fields in all bodies.
7. Run the `verify` skill. Update `docs/architecture.md` (status diagram / API
   table) if statuses or endpoints changed.
