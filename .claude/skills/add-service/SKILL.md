---
name: add-service
description: Add a new Go service module to the workspace (module skeleton, go.work, Taskfile loops, Dockerfiles, compose, mockery, docs). Use when a task introduces a new microservice such as AssemblyService.
---

# Add a service module

Reference: `assembly/` (added in week 5).

1. **Skeleton** — `<svc>/{cmd/main.go, internal/app/container.go, pkg/service, pkg/api|pkg/kafka}`,
   `go.mod` with `module github.com/omaigo88/<svc>` and
   `replace github.com/omaigo88/shared => ../shared`. `cmd/main.go`:
   `signal.NotifyContext`, build container, run, graceful close.
2. **Workspace** — add `./<svc>` to `go.work` `use (...)` (keep sorted) and to its TODO comment.
3. **Taskfile** — add the module to `MODULES`, the `build` and `lint` package
   lists, every `for mod in …` loop (test, test:coverage, coverage merge), and a
   `run:<svc>` task.
4. **Docker** — `<svc>/Dockerfile` copying the pattern of `assembly/Dockerfile`;
   add `COPY <svc>/go.mod <svc>/go.sum ./<svc>/` to **every other Dockerfile** too.
5. **Compose** — service in `deploy/compose/docker-compose.yaml` with env vars,
   `depends_on` (healthchecks / `kafka-init`), port only if it serves traffic.
   Postgres per service if it owns data (own `<svc>.env`, own host port, migrations dir, `migrate:<svc>:*` tasks).
6. **Mocks** — `.mockery.yaml` entries for packages that declare interfaces; `task mocks:gen`.
7. **Tests** — unit tests for service and adapters; e2e scenario proving the
   service participates in the flow; update the header comment in `scripts/e2e_test.sh`.
8. `cd <svc> && go mod tidy`; run the `verify` skill.
9. **Docs** — service table in `CLAUDE.md`, component + sequence diagrams in
   `docs/architecture.md`, ports list in `.claude/rules/infra.md`.
