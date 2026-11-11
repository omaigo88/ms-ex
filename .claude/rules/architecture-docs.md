# Keep `docs/architecture.md` in sync

`docs/architecture.md` is the source of truth for how the system fits
together. **Any change that alters the architecture must update it in the same
change set** — not as a follow-up, and not only when asked. A task with an
architectural change is not done until the doc matches the code.

## What counts as an architectural change

- A service or module is added, removed, renamed, or changes its port/transport.
- A synchronous call between services is added/removed (new gRPC method used
  by another service, new HTTP endpoint).
- A Kafka topic, event message, event field with business meaning, producer,
  consumer or consumer group is added/changed.
- The order lifecycle changes: a new status, transition, or rule about which
  operations are allowed in which status.
- Inventory stock/reservation semantics change (what create/cancel/assemble do
  to `reserved` / `stock_quantity`).
- Persistence: a new database, table owned by a service, transaction boundary,
  or locking strategy.
- Delivery/consistency semantics: retries, idempotency, outbox, sagas,
  ordering guarantees.
- Deployment topology: Compose services, infrastructure components, public
  ports, how services find each other.
- Error contract: a new error that maps to a distinct HTTP/gRPC status.
- A known limitation is introduced or removed.

Pure refactors inside a layer, test-only changes, and dependency bumps do not
need a doc update.

## How to update

- Edit the affected sections only; keep the existing structure: overview →
  component diagram → order lifecycle → event flow (+ events table, delivery
  semantics) → create/cancel → transactions and concurrency → deployment →
  error mapping → known limitations.
- Diagrams are Mermaid in the markdown — update the diagram *and* the prose/tables
  around it. Every name in a diagram (service, rpc, topic, status, table) must
  exist in the code under that exact name.
- Verify facts against the code before writing (read the service method, the
  compose file, the proto), don't describe the intended design from memory.
- If the change also affects the summary in `CLAUDE.md` (services table,
  order flow, statuses, known limitations) or a rule in `.claude/rules/`
  (ports, topics, error mapping), update those too.
- In the final report to the user, state which sections of
  `docs/architecture.md` were changed — or explicitly that no update was needed and why.
