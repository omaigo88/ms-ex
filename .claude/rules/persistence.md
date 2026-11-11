---
paths:
  - "*/pkg/repository/**"
  - "*/pkg/service/**"
  - "*/migrations/**"
  - "*.env"
---

# Persistence, transactions, locking

## Transactions

- Order and Inventory use avito-tech go-transaction-manager (pgx v5 driver).
  Services wrap read-modify-write in `trManager.Do(ctx, func(ctx) error {...})`.
- Repositories get the connection with `trmpgx.DefaultCtxGetter.DefaultTrOrDB(ctx, pool)`
  so they work inside or outside a transaction without knowing which.
- Tests and in-memory wiring use `service.NewNoopTrManager()`.

## Row locks (`SELECT … FOR UPDATE`)

- Every read-modify-write reads through `GetForUpdate` / `GetPartForUpdate`
  inside the transaction — never `Get` then `Save` across separate statements.
- When locking several rows, **sort the keys first** (Inventory sorts part UUIDs)
  so concurrent requests always lock in the same order and can't deadlock.
- In-memory repositories implement `…ForUpdate` as a plain read (guarded by a mutex).
- Side effects that must not be rolled back with the DB (Kafka publish) happen
  *after* `trManager.Do` returns successfully.

## Migrations (goose, `<svc>/migrations/NNNNN_snake_case.sql`)

- Sequential 5-digit numbers; each file has `-- +goose Up` and a working `-- +goose Down`.
- Never edit a migration that has been committed — add a new one.
- Schema changes on existing data use **expand → backfill → contract** as
  separate migrations (see `order/migrations/00002..00004` for `user_uuid`):
  add nullable column → fill existing rows → add `NOT NULL`/constraints.
- Apply with `task migrate:<svc>:up`; DB URIs come from `order.env` / `inventory.env`.
- Inventory seed data lives in `inventory/migrations/00001_*` and is referenced
  by UUID in `scripts/e2e_test.sh` — keep them in sync.
