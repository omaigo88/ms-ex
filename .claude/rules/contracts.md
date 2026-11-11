---
paths:
  - "shared/proto/**"
  - "shared/api/**"
  - "shared/pkg/proto/**"
  - "shared/pkg/openapi/**"
---

# Contracts (protobuf + OpenAPI)

## Protobuf (`shared/proto`, buf v2)

- Generated into `shared/pkg/proto/<pkg>/v1` by `task proto:gen` (also runs `buf format`).
  `task proto:lint` must pass: STANDARD + `COMMENT_*` rules — **every service,
  rpc, message, field, enum and enum value needs a comment**.
- Packages are versioned (`inventory.v1`, `payment.v1`, `events.v1`);
  `go_package = "github.com/omaigo88/shared/pkg/proto/<pkg>/v1;<pkg>v1"`.
- Backward compatibility: never renumber or reuse field numbers; remove a field
  by `reserved`-ing its number and name. Each rpc has its own `XRequest`/`XResponse`
  messages, even if empty.
- UUIDs travel as `string`; parse and validate at the receiving `api` layer.

## OpenAPI (`shared/api/order/v1`, ogen)

- Split spec: `order.openapi.yaml` → `paths/`, `components/` (DTOs, `enums/`,
  `errors/`, `responses/`), `params/`. Edit the component file, not a copy.
- Generate with `task ogen:gen` into `shared/pkg/openapi/order/v1` (package `orderv1`).
- New request fields that the service needs are `required` with a `format`
  (`uuid`, enums) so ogen validates them and the handler never sees bad input.
- New enum value (e.g. an order status) → update the enum YAML, the repository
  status constant, the handler mapping, and the e2e assertions.

## Never

Hand-edit `shared/pkg/proto/**` or `shared/pkg/openapi/**` — regenerate.
