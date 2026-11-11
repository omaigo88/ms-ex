---
name: add-migration
description: Create goose SQL migrations for Order or Inventory, including the expand-backfill-contract sequence for changes to tables with existing data, and wire the new column through the repository. Use when adding/changing columns, tables, indexes or seed data.
---

# Add a migration

1. Next number: `ls <svc>/migrations` → `NNNNN+1`, name `NNNNN_snake_case.sql`.
   Never modify an already committed migration.
2. Template:
   ```sql
   -- +goose Up
   -- <why, one line>
   ALTER TABLE ...;

   -- +goose Down
   ALTER TABLE ...;
   ```
   Down must actually revert Up (for a data-only backfill, `SELECT 1;` is fine).
3. **Column on a table with data** — three migrations (see `order/migrations/00002..00004`):
   1. *expand*: add the column nullable (or with a default);
   2. *backfill*: `UPDATE … SET col = <placeholder> WHERE col IS NULL`;
   3. *contract*: `ALTER COLUMN col SET NOT NULL` / add constraints.
4. **Code**: record struct / model field, every `SELECT` column list
   (`select…Columns` constants), `INSERT`/upsert, scan order; in-memory
   repository too. If the field is exposed, follow `change-order-api`.
5. Check locally:
   ```bash
   task up
   task migrate:<svc>:up && task migrate:<svc>:down && task migrate:<svc>:up
   task migrate:<svc>:status
   task down
   ```
6. If seed data changed, update UUIDs/expectations in `scripts/e2e_test.sh`.
7. Run the `verify` skill (e2e required).
