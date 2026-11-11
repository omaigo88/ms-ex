---
name: verify
description: Run the project's full verification ladder (tidy, format, lint, unit, API, coverage, e2e) and report results. Use before declaring a task done, before committing, or when the user asks to "check everything" / "run all tests".
---

# Verify

Run from the repo root. Stop at the first failing step, fix it, and re-run from
that step. Never report a step as passed without having run it in this session.

1. **Generated code is current** (only if contracts/interfaces changed):
   `task gen && task mocks:gen`, then `git status` — no unexpected diffs.
2. **Modules are tidy**:
   ```bash
   for m in shared inventory payment order assembly; do
     (cd $m && GOWORK=off go mod tidy -diff >/dev/null || echo "untidy: $m")
   done
   ```
3. `task format` — then check `git diff` didn't change files you didn't touch.
4. `task build`
5. `task lint` — must end with `0 issues.`
6. `task test` — every package `ok`, no `DATA RACE`.
7. `task test:api`
8. `task test:coverage` — must meet the 40% threshold.
9. **E2E** — required if the change touches migrations, compose/Dockerfiles,
   Kafka, cross-service flow, or anything visible through the HTTP API:
   ```bash
   task down >/dev/null 2>&1
   task test:e2e > "$TMPDIR/e2e.log" 2>&1; echo "exit=$?"
   grep -E '^(PASS|FAIL|ERROR|==>)' "$TMPDIR/e2e.log"
   ```
   The script dumps all container logs on failure, so grep for `FAIL` first and
   only then read the logs of the relevant service. Confirm the stack is down
   afterwards (`docker ps --filter name=ms-ex -q` is empty).

10. **Architecture doc** — if the change is architectural (see
    `.claude/rules/architecture-docs.md`), confirm `docs/architecture.md` was
    updated in this change set (`git diff --stat -- docs/architecture.md`) and
    that every diagram name still exists in the code. If it wasn't, update it
    before reporting.

## Report

A short table: step → result (pass / fail / skipped + why). Include the exact
failing output for any failure. Don't commit unless the user asked.
