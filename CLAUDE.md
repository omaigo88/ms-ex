# ms-ex — "order a spaceship" microservices

Go 1.26 workspace of four services plus a shared module. Read
`docs/architecture.md` for diagrams; this file is the working contract.

## Services

| Module      | Transport                 | Owns                                   |
|-------------|---------------------------|----------------------------------------|
| `order`     | HTTP `:8080` (ogen)       | orders (Postgres), the only public API |
| `inventory` | gRPC `:50051`             | parts catalog + stock (Postgres)       |
| `payment`   | gRPC `:50052`             | payments (stateless)                   |
| `assembly`  | Kafka only (no port)      | simulated ship assembly (stateless)    |
| `shared`    | —                         | proto/OpenAPI contracts, generated code, `pkg/kafka` |

Order flow: `POST /orders` (Inventory `ReserveParts`) → `POST /orders/{id}/pay`
(Payment `PayOrder`, status `PAID`, publish `OrderPaid`) → AssemblyService
(5–15 s) publishes `ShipAssembled` → Order calls Inventory `CommitParts` →
status `ASSEMBLED`. Cancel only from `PENDING_PAYMENT` (Inventory `ReleaseParts`).

Statuses: `PENDING_PAYMENT`, `PAID`, `ASSEMBLED`, `CANCELLED`. Pay/Cancel on a
non-pending order → `ErrOrderAlreadyPaid` / `ErrOrderCancelled` /
`ErrOrderAssembled` → HTTP 409, state unchanged.

## Layout (every service)

```
<svc>/cmd/main.go            entrypoint: config, signals, graceful shutdown
<svc>/internal/app/          dependency wiring (container) — the only place that builds concrete types
<svc>/pkg/api/               transport adapter: wire ⇄ domain types, error → status code
<svc>/pkg/service/           business logic; depends only on interfaces it declares itself
<svc>/pkg/repository/        Postgres + in-memory implementations
<svc>/pkg/domain/            (inventory) aggregates with invariants
<svc>/pkg/kafka/             (order, assembly) Kafka adapters around the service
<svc>/migrations/            goose SQL migrations
```

## Commands (go-task; never call tools directly when a task exists)

| Task | What |
|------|------|
| `task setup` | install all pinned tools into `./bin` |
| `task gen` / `proto:gen` / `ogen:gen` / `mocks:gen` | regenerate code |
| `task format` | gofumpt + gci |
| `task lint` | golangci-lint v2 over all modules |
| `task test` | unit tests, `-race`, all modules |
| `task test:api` | `order/tests` — HTTP API against in-memory wiring, no Docker |
| `task test:coverage` | coverage, threshold 40% |
| `task up` / `stop` / `down` | whole stack via `deploy/compose/docker-compose.yaml` (project `ms-ex`) |
| `task migrate:{order,inventory}:{up,down,status}` | goose |
| `task test:e2e` | `scripts/e2e_test.sh`: real stack + Kafka chain, tears down after |
| `task run:{order,inventory,payment,assembly}` | run one service on the host |

Kafka UI: http://localhost:8090. Host Kafka listener: `localhost:9094`.

## Definition of done

1. Generated code regenerated if contracts changed (`task gen`, `task mocks:gen`).
2. `task format`, `task lint` → 0 issues.
3. `task test` and `task test:api` pass.
4. Changes to infra, migrations, Kafka or cross-service flow → `task test:e2e` passes.
5. `go mod tidy -diff` is empty in every touched module.
6. **Architectural change → `docs/architecture.md` updated in the same change**
   (services, inter-service calls, Kafka events, statuses, persistence/locking,
   deployment, error contract, known limitations). See `.claude/rules/architecture-docs.md`.

Report results faithfully — if a step was skipped or failed, say so.

## Hard rules

- **Never hand-edit generated code**: `shared/pkg/proto/**`, `shared/pkg/openapi/**`,
  `**/mocks/**`. Change the source (`.proto`, OpenAPI YAML, interface) and regenerate.
- **Never edit an applied migration** — add a new one.
- Everything in the repo (code, comments, docs, commit messages) is in **English**.
- Commit/push only when explicitly asked. Never force-push without being asked.
- Keep the e2e script working: every behavior change that's visible via the API gets a scenario.
- Keep `docs/architecture.md` true: never finish an architectural change without updating it.

## Where the details live

- `.claude/rules/` — conventions, auto-loaded by path (Go style, layering,
  testing, persistence, Kafka, contracts, infra); `architecture-docs.md` is
  always loaded.
- `.claude/skills/` — step-by-step playbooks: `verify`, `add-grpc-method`,
  `add-kafka-event`, `add-migration`, `change-order-api`, `add-service`.

## Known limitations (intentional, don't "fix" silently)

- No transactional outbox: `OrderPaid` is published after the DB commit; a
  publish failure is logged only.
- No cross-service saga/compensation.
- Order holds a row lock across gRPC calls in Pay/Cancel/MarkAssembled.
