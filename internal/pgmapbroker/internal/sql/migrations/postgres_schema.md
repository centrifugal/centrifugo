# PostgreSQL MapBroker Schema Management

## Overview

`EnsureSchema()` manages the PostgreSQL schema for MapBroker automatically. It creates **both** JSONB and BYTEA schema variants in a single call (regardless of `BinaryData` config), uses integer-based versioning, and supports forward migrations.

The PostgreSQL MapBroker lives in Centrifugo at `internal/pgmapbroker/`.

## What EnsureSchema handles automatically

| Change type | Mechanism | Migration needed? |
|---|---|---|
| New table | `CREATE TABLE IF NOT EXISTS` | No |
| New index | `CREATE INDEX IF NOT EXISTS` | No |
| Function body change | `CREATE OR REPLACE FUNCTION` | No |
| New function param with DEFAULT | `CREATE OR REPLACE FUNCTION` | No |
| New column on existing table | -- | **Yes** (`ALTER TABLE ADD COLUMN IF NOT EXISTS`) |
| Column type change | -- | **Yes** (`ALTER TABLE ALTER COLUMN TYPE`) |
| Function signature change (param/return types) | -- | **Yes** (`DROP FUNCTION` + `CREATE`) |
| Drop column | -- | **Yes** (two-phase: stop using, then drop) |

## Schema versioning

- Version is stored in `cf_map_schema_version` (and `cf_binary_map_schema_version`).
- On startup: if version matches and a probe query succeeds, all DDL is skipped (fast path).
- Fresh installs: DDL creates latest schema, version set to current. Migrations are skipped.
- Upgrades: DDL re-applied (idempotent), then migrations run in order from `dbVersion+1` to `schemaVersion`.

## Rolling deploy rules

1. **Additive changes are safe.** New columns with DEFAULT, new function params with DEFAULT -- old nodes ignore them, new nodes use them.
2. **Destructive changes need two phases.** Phase 1: Deploy new code that stops using the old column. Phase 2: Deploy migration that drops it.
3. **Function signature changes need coordination.** Options: accept brief errors during DROP+CREATE (<1s), or use a new function name (zero-downtime).
4. **EnsureSchema runs once per startup.** Concurrent runs are safe (idempotent DDL + retry on deadlock).
5. **NumShards changes require full restart** (not rolling). Concurrent nodes with different NumShards race on shard_lock population.
6. **Rollback (downgrade) is safe.** EnsureSchema overwrites functions to the older version. Extra columns from newer migrations remain but are ignored by older code. No data loss.

## Manual migration path

1. **Fresh install:** Apply `internal/pgmapbroker/internal/sql/schema_all.sql`.
2. **Check version:** `SELECT schema_version FROM cf_map_schema_version WHERE id = 1`.
3. **Apply migrations in order:** files in `internal/pgmapbroker/internal/sql/migrations/`.
4. **EnsureSchema optional:** Safe to keep (no-op when version matches), safe to disable.

## Adding a migration (developer workflow)

1. Bump `schemaVersion` in `internal/pgmapbroker/pgmapbroker.go`.
2. Create `internal/pgmapbroker/internal/sql/migrations/NNN.sql` -- explicit SQL for both prefixes, idempotent.
3. Add entry to `schemaMigrations` map (embed the file).
4. Update `schema.sql` template to reflect the final state (fresh installs get everything).
5. `make pg-schemas` to regenerate.
6. **Invariant:** fresh install via DDL + upgrade via migrations must produce identical schema.

## Migration file conventions

Migration files are plain SQL targeting both prefixes explicitly:

```sql
-- Migration 002: Add foo column
ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS foo TEXT;
ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS foo TEXT;
```

No template placeholders in migration files -- explicit is safer for production migrations.

**Requirements on migration authors:**
- All migrations MUST be idempotent (`ADD COLUMN IF NOT EXISTS`, etc.)
- All migrations MUST be backwards-compatible (old code continues working after migration)
- Each migration targets BOTH `cf_map_*` and `cf_binary_map_*` prefixes
