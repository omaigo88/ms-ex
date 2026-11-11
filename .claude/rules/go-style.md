---
paths:
  - "**/*.go"
---

# Go style

- Formatting is gofumpt + gci with sections `standard`, `default`,
  `prefix(github.com/omaigo88)` — run `task format`, don't hand-arrange imports.
- Every exported identifier and every package has a doc comment starting with
  its name. Comments explain *why* (locking order, idempotency, at-least-once),
  not what the next line does.
- Errors:
  - Package-level sentinels (`var ErrX = errors.New("lowercase message")`), each
    with a doc comment. Callers use `errors.Is`, never string matching.
  - Wrap with context: `fmt.Errorf("save order: %w", err)`.
  - Sentinel messages are user-visible (the Order API returns them in 409/404
    bodies and the e2e script asserts on them) — changing one is an API change.
- Logging: `log/slog` with key/value pairs (`"order_uuid", id`). No `fmt.Println`, no `log.Printf`.
- `context.Context` is the first parameter of anything that does I/O; never store it in a struct.
- Configuration via env vars read only in `internal/app` (with defaults for host runs), never in `pkg/`.
- Constructors are `NewX(...)` returning the concrete type or the interface the
  package already exposes; dependencies are passed in, not created inside.
- Prefer small consumer-owned interfaces (declared where used, e.g.
  `order/pkg/service.InventoryClient`) over importing a provider's interface.
- No `panic` outside `main`/wiring; no global mutable state.
