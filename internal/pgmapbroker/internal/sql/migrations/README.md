# Schema Migrations

Migration files for PostgreSQL MapBroker schema upgrades.

The PostgreSQL MapBroker lives in Centrifugo at `internal/pgmapbroker/`.

## Conventions

- Files named `NNN.sql` where NNN is the target version (e.g., `002.sql`).
- Version 1 is the baseline (applied via full DDL in `schema.sql`).
- Each migration MUST be idempotent (use `IF NOT EXISTS`, `IF EXISTS`, etc.).
- Each migration MUST target BOTH prefixes (`cf_map_*` and `cf_binary_map_*`).
- No template placeholders -- use explicit table names.

## Example

```sql
-- Migration 002: Add foo column
ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS foo TEXT;
ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS foo TEXT;
```

See `postgres_schema.md` for the full developer workflow.
