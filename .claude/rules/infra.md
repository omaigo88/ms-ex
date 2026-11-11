---
paths:
  - "deploy/**"
  - "**/Dockerfile"
  - "Taskfile.yaml"
  - "go.work"
  - "**/go.mod"
  - "scripts/**"
  - ".github/**"
---

# Infrastructure and tooling

- One compose file, `deploy/compose/docker-compose.yaml`, project name `ms-ex`.
  Containers are `ms-ex-<service>-1` (the e2e script relies on these names).
  Use `task up` / `stop` / `down`; don't add parallel compose files.
- Services that need Kafka `depends_on: kafka-init: service_completed_successfully`;
  services that need Postgres depend on its healthcheck.
- Every Dockerfile builds from the repo root in workspace mode and must
  `COPY` **every** module's `go.mod`/`go.sum` (`order inventory payment assembly shared`)
  before `go mod download`. Runtime image: alpine, non-root `appuser`.
- Tool versions are pinned in `Taskfile.yaml` vars and installed into `./bin`
  (`task setup`); never use globally installed linters/generators.
- Modules are listed in several places that must stay in sync: `go.work`,
  Taskfile `MODULES` var and the `for mod in …` loops (build, lint, test,
  test:coverage, coverage merge), every Dockerfile, `.mockery.yaml`.
- After dependency changes run `go mod tidy` in each touched module
  (`GOWORK=off go mod tidy -diff` must be empty); gopls reports untidy modules
  even when the workspace builds.
- CI (`.github/workflows/ci.yml`) calls the same `task` targets as local runs —
  add new checks as tasks first, then call them from CI; never duplicate
  commands inline. Jobs: `lint` (build, golangci-lint, proto lint, format
  diff), `generated` (gen + mocks diff, `go mod tidy -diff`), `unit-tests`
  (`test:coverage`), `api-tests`. The `CI passed` job aggregates them and is
  the only required check in the ruleset — add a new job to its `needs`, don't
  rename it. A new module also goes into the tidy loop there.
- Ports: Order 8080, Inventory 50051, Payment 50052, order-postgres 5432,
  inventory-postgres 5433, Kafka 9094 (host) / 9092 (in-network), Kafka UI 8090.
