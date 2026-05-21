-- PostgreSQL Controller schema template.
-- Placeholders: __PREFIX__ → e.g. "cf_" (includes trailing underscore).
--
-- Tables:
--   __PREFIX__controller_messages        — control message outbox (partitioned by created_at, daily)
--   __PREFIX__controller_shard_lock      — per-shard serialization lock rows
--   __PREFIX__controller_schema_version  — schema version tracking
--
-- Functions:
--   __PREFIX__controller_publish(BYTEA, TEXT, INTEGER) — shard lock + INSERT + NOTIFY

-- Messages outbox table — always partitioned by created_at (daily).
CREATE TABLE IF NOT EXISTS __PREFIX__controller_messages (
    id          BIGSERIAL,
    shard_id    SMALLINT NOT NULL DEFAULT 0,
    node_id     TEXT NOT NULL DEFAULT '',
    data        BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS __PREFIX__controller_messages_shard_id_idx
    ON __PREFIX__controller_messages (shard_id, id);

-- Per-shard serialization lock. Rows populated by EnsureSchema (0..NumShards-1).
-- The publish function acquires a FOR UPDATE lock on the relevant row before
-- INSERT, ensuring BIGSERIAL id assignment matches commit order within a shard.
CREATE TABLE IF NOT EXISTS __PREFIX__controller_shard_lock (
    shard_id SMALLINT PRIMARY KEY
);

-- Schema version tracking.
CREATE TABLE IF NOT EXISTS __PREFIX__controller_schema_version (
    id              INTEGER PRIMARY KEY,
    schema_version  INTEGER NOT NULL
);
INSERT INTO __PREFIX__controller_schema_version (id, schema_version) VALUES (1, 1)
    ON CONFLICT (id) DO NOTHING;

-- Publish function: shard lock + INSERT + NOTIFY.
-- Acquires a FOR UPDATE lock on the shard_lock row for the computed shard,
-- serializing concurrent publishes within the same shard. This ensures the
-- BIGSERIAL id order matches commit order — no gaps for cursor-based polling.
CREATE OR REPLACE FUNCTION __PREFIX__controller_publish(
    p_data BYTEA,
    p_node_id TEXT DEFAULT '',
    p_num_shards INTEGER DEFAULT NULL
) RETURNS BIGINT AS $$
DECLARE
    v_id BIGINT;
    v_shard_id INTEGER;
BEGIN
    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL OR p_num_shards <= 0 THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__controller_shard_lock;
    END IF;

    -- All messages go to shard 0 when num_shards=1. With multiple shards,
    -- distribute by hash of node_id (targeted) or round-robin via txid (broadcast).
    IF p_num_shards <= 1 THEN
        v_shard_id := 0;
    ELSIF p_node_id != '' THEN
        v_shard_id := abs(hashtext(p_node_id)) % p_num_shards;
    ELSE
        v_shard_id := (txid_current() % p_num_shards)::INTEGER;
    END IF;

    -- Serialize within shard.
    PERFORM 1 FROM __PREFIX__controller_shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    INSERT INTO __PREFIX__controller_messages (shard_id, node_id, data)
    VALUES (v_shard_id, p_node_id, p_data)
    RETURNING id INTO v_id;

    PERFORM pg_notify('__PREFIX__controller_notify', '');
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;
