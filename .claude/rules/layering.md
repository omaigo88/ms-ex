---
paths:
  - "*/pkg/**/*.go"
  - "*/internal/**/*.go"
  - "*/cmd/**/*.go"
---

# Layering

Dependency direction is strictly `cmd → internal/app → pkg/{api,kafka} → pkg/service → pkg/{domain,repository}`.

- **`pkg/api`** (HTTP handler / gRPC server) — converts wire types (ogen / proto)
  to service inputs and back, and maps service/domain errors to status codes in
  one place (`switch` with `errors.Is`). No business decisions here.
  - Order HTTP: not found → 404, status conflicts / out of stock → 409,
    validation → 400 (mostly done by ogen), everything else → 500.
  - Inventory gRPC: invalid UUID → `InvalidArgument`, not found → `NotFound`,
    invariant violations (`ErrInsufficientStock`, `ErrIncompatibleParts`,
    `ErrReservationUnderflow`) → `FailedPrecondition`, else `Internal`.
- **`pkg/kafka`** — the async equivalent of `pkg/api`: decode the event, call the
  service, return its error. No business logic.
- **`pkg/service`** — business rules and orchestration. Declares the interfaces
  it needs (`Repository`, `InventoryClient`, `TrManager`, publishers). Must not
  import ogen/HTTP types or Kafka/sarama. Proto *client* types are allowed only
  through those interfaces.
- **`pkg/domain`** (inventory) — aggregates guard their own invariants
  (`Part.Reserve/Release/Commit`); state changes only through methods, fields unexported.
- **`pkg/repository`** — persistence only; returns `ErrNotFound`, never HTTP/gRPC errors.
  Every repository has a Postgres and an in-memory implementation with the same behavior.
- **`internal/app`** — the only place that reads env vars, opens pools/connections,
  and builds concrete types. Owns close order (reverse of construction).

When adding a dependency to a service: add a method to the consumer-owned
interface, implement it, wire it in `internal/app`, regenerate mocks.
