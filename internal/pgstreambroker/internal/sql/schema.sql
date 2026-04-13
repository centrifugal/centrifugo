-- ============================================================================
-- PostgreSQL StreamBroker Schema and Functions (template).
-- Auto-created by EnsureSchema(). All statements are idempotent.
-- Placeholders are replaced at runtime by PostgresStreamBroker.renderSchema:
--   __DATA_TYPE__ → JSONB (default) or BYTEA (when BinaryData is true)
--   __PREFIX__    → <TablePrefix>_stream_ or <TablePrefix>_binary_stream_
--                   where TablePrefix is user-configurable (default "cf").
-- EnsureSchema renders this template once per variant (JSONB + binary) and
-- executes both, so a broker always creates both sets of tables regardless
-- of the BinaryData runtime flag.
-- ============================================================================

-- ============================================================================
-- Stream Table (Publications + Joins/Leaves)
--
-- The stream table is always partitioned by created_at (daily). The composite
-- primary key (id, created_at) is required because Postgres mandates that the
-- partition key be part of every unique constraint. Initial partitions
-- (today + lookahead) are created via pgoutbox.Partitioner.EnsureLookaheadPartitions
-- after this DDL runs; the partition retention worker drops old partitions.
--
-- The kind column distinguishes:
--   0 = publication (the common case; History() filters WHERE kind = 0)
--   1 = join event (PublishJoin)
--   2 = leave event (PublishLeave)
-- Joins and leaves piggyback on partition retention — no special cleanup pass.
-- ============================================================================

CREATE TABLE IF NOT EXISTS __STREAM_TABLE__ (
    id              BIGSERIAL,
    channel         TEXT NOT NULL,
    channel_offset  BIGINT NOT NULL,
    epoch           TEXT NOT NULL DEFAULT '',
    kind            SMALLINT NOT NULL DEFAULT 0,
    data            __DATA_TYPE__,
    tags            JSONB,
    client_id       TEXT,
    user_id         TEXT,
    conn_info       __DATA_TYPE__,
    chan_info       __DATA_TYPE__,
    key             TEXT,
    prev_data       __DATA_TYPE__,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    shard_id        SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Publications-only partial index. History() reads filter WHERE kind = 0,
-- and the delta prev_data lookup also filters WHERE kind = 0. A partial
-- index keeps the index small and the planner happy without scanning over
-- join/leave rows.
CREATE INDEX IF NOT EXISTS __STREAM_TABLE___channel_offset_idx
    ON __STREAM_TABLE__ (channel, channel_offset)
    WHERE kind = 0;

-- Outbox worker batch fetch — polls all kinds (publications, joins, leaves)
-- and switches in Go.
CREATE INDEX IF NOT EXISTS __STREAM_TABLE___shard_id_idx
    ON __STREAM_TABLE__ (shard_id, id);

-- Time-based scans (cleanup, retention, audit). Applies to all kinds.
CREATE INDEX IF NOT EXISTS __STREAM_TABLE___created_at_idx
    ON __STREAM_TABLE__ (created_at);

-- ============================================================================
-- Channel Metadata Table
--
-- Tracks per-channel state needed for publish ordering, recovery, and the
-- read-time TTL filter:
--
--   top_offset       — last assigned channel_offset (incremented on publish)
--   epoch            — monotonic epoch string; new on first publish, persists
--                      until the channel is forgotten via meta_expires_at
--   version          — for version-based publish suppression
--   version_epoch    — companion to version (epoch reset triggers re-comparison)
--   history_ttl      — from PublishOptions.HistoryTTL; refreshed on each publish.
--                      Used by History() reads for the read-time TTL filter.
--   history_size     — from PublishOptions.HistorySize; refreshed on each publish.
--                      Used by History() reads to clamp the result window.
--   meta_expires_at  — from PublishOptions.HistoryMetaTTL (with 3-tier fallback);
--                      always non-NULL; when it passes, the meta cleanup pass
--                      deletes the row and the channel is forgotten.
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__meta (
    channel         TEXT PRIMARY KEY,
    top_offset      BIGINT NOT NULL DEFAULT 0,
    epoch           TEXT NOT NULL DEFAULT '',
    version         BIGINT NOT NULL DEFAULT 0,
    version_epoch   TEXT NOT NULL DEFAULT '',
    history_ttl     INTERVAL,
    history_size    INTEGER,
    meta_expires_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS __PREFIX__meta_expires_idx
    ON __PREFIX__meta (meta_expires_at)
    WHERE meta_expires_at IS NOT NULL;

-- Per-table autovacuum tuning: meta sees two writes per publish (lock + increment)
-- plus one write per History() read (TTL refresh). Default autovacuum thresholds
-- can lag behind dead-tuple production for high-traffic channels.
ALTER TABLE __PREFIX__meta SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_vacuum_threshold = 100,
    autovacuum_analyze_scale_factor = 0.05
);

-- ============================================================================
-- Idempotency Table
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__idempotency (
    channel         TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    result_offset   BIGINT NOT NULL,
    result_id       BIGINT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (channel, idempotency_key)
);

CREATE INDEX IF NOT EXISTS __PREFIX__idempotency_expires_idx
    ON __PREFIX__idempotency (expires_at);

ALTER TABLE __PREFIX__idempotency SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_vacuum_threshold = 100,
    autovacuum_analyze_scale_factor = 0.05
);

-- ============================================================================
-- Shard Lock Table
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__shard_lock (
    shard_id SMALLINT PRIMARY KEY
);

-- ============================================================================
-- Schema Version
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__schema_version (
    id              INTEGER PRIMARY KEY,
    schema_version  INTEGER NOT NULL
);
INSERT INTO __PREFIX__schema_version (id, schema_version) VALUES (1, 1)
    ON CONFLICT (id) DO NOTHING;

-- ============================================================================
-- Functions
-- ============================================================================

-- __PREFIX__publish: atomic publish with version, idempotency, and TTL handling.
--
-- Lock order: shard → meta. Publishes on the same shard serialize via the
-- shard_lock row, ensuring BIGSERIAL id assignment matches commit order
-- within each shard (the invariant the outbox worker relies on).
--
-- Defensive clamp: effective_meta_ttl = GREATEST(p_meta_ttl, p_history_ttl)
-- so the meta row always survives at least as long as the recoverable history,
-- preventing the foot-gun of metaTTL < historyTTL.
--
-- Suppression: idempotency key match or version-too-low return early without
-- incrementing top_offset (the publish is treated as a no-op except for the
-- returned position).
--
-- The function uses INSERT ON CONFLICT DO UPDATE with a no-op write to acquire
-- the meta row lock atomically — the DO NOTHING + SELECT FOR UPDATE pattern
-- has a race window where concurrent cleanup could delete the row between
-- statements.
CREATE OR REPLACE FUNCTION __PREFIX__publish(
    p_channel TEXT,
    p_data __DATA_TYPE__,
    p_tags JSONB DEFAULT NULL,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_key TEXT DEFAULT NULL,
    p_history_ttl INTERVAL DEFAULT NULL,
    p_history_size INTEGER DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_version BIGINT DEFAULT NULL,
    p_version_epoch TEXT DEFAULT NULL,
    p_use_delta BOOLEAN DEFAULT FALSE,
    p_num_shards INTEGER DEFAULT NULL
) RETURNS TABLE(
    out_result_id BIGINT,
    out_channel_offset BIGINT,
    out_epoch TEXT,
    out_suppressed BOOLEAN,
    out_suppress_reason TEXT
) AS $$
DECLARE
    v_offset BIGINT;
    v_id BIGINT;
    v_epoch TEXT;
    v_prev_ver BIGINT;
    v_prev_ver_epoch TEXT;
    v_prev_data __DATA_TYPE__;
    v_shard_id INTEGER;
    v_effective_meta_ttl INTERVAL;
    v_cached_offset BIGINT;
BEGIN
    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;

    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    -- Per-shard serialization lock (lock order: shard → meta).
    PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    -- Defensive clamp: effective_meta_ttl = GREATEST(p_meta_ttl, p_history_ttl).
    -- COALESCE NULL → '0'::interval before GREATEST, then convert '0' back to NULL.
    v_effective_meta_ttl := GREATEST(
        COALESCE(p_meta_ttl, '0'::interval),
        COALESCE(p_history_ttl, '0'::interval)
    );
    IF v_effective_meta_ttl = '0'::interval THEN
        v_effective_meta_ttl := NULL;
    END IF;

    -- Atomic insert-or-lock meta. ON CONFLICT DO UPDATE with a no-op write
    -- acquires the row lock and returns the pre-update state in a single
    -- statement, avoiding the DO NOTHING + SELECT FOR UPDATE race.
    INSERT INTO __PREFIX__meta AS m (channel, top_offset, epoch, updated_at)
    VALUES (p_channel, 0, substr(md5(random()::text || random()::text), 1, 8), NOW())
    ON CONFLICT (channel) DO UPDATE
        SET updated_at = m.updated_at
    RETURNING m.top_offset, m.epoch, m.version, m.version_epoch
    INTO v_offset, v_epoch, v_prev_ver, v_prev_ver_epoch;

    -- Idempotency check.
    IF p_idempotency_key IS NOT NULL THEN
        SELECT result_offset INTO v_cached_offset
        FROM __PREFIX__idempotency
        WHERE channel = p_channel AND idempotency_key = p_idempotency_key
          AND expires_at > NOW();
        IF FOUND THEN
            RETURN QUERY SELECT NULL::BIGINT, v_cached_offset, v_epoch, TRUE, 'idempotency'::TEXT;
            RETURN;
        END IF;
    END IF;

    -- Version check (publication-level, not key-based since stream broker
    -- has no key state).
    IF p_version IS NOT NULL AND p_version > 0 THEN
        IF (p_version_epoch IS NULL OR p_version_epoch = '' OR p_version_epoch = v_prev_ver_epoch)
           AND v_prev_ver >= p_version THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'version'::TEXT;
            RETURN;
        END IF;
    END IF;

    -- Delta lookup: fetch the most recent publication's data for this channel.
    -- Order by channel_offset (not id) so the partial index can serve it.
    -- Within a single channel under the shard lock, channel_offset and id
    -- are monotonic together, so the result is identical.
    IF p_use_delta THEN
        SELECT data INTO v_prev_data
        FROM __STREAM_TABLE__
        WHERE channel = p_channel AND kind = 0
        ORDER BY channel_offset DESC LIMIT 1;
    END IF;

    -- Increment offset + refresh meta TTLs and HistorySize. The COALESCE
    -- pattern preserves existing values when the parameter is NULL.
    UPDATE __PREFIX__meta AS m
       SET top_offset = m.top_offset + 1,
           version = COALESCE(p_version, m.version),
           version_epoch = COALESCE(p_version_epoch, m.version_epoch),
           history_ttl = COALESCE(p_history_ttl, m.history_ttl),
           history_size = COALESCE(p_history_size, m.history_size),
           meta_expires_at = CASE
               WHEN v_effective_meta_ttl IS NOT NULL THEN NOW() + v_effective_meta_ttl
               ELSE m.meta_expires_at
           END,
           updated_at = NOW()
     WHERE m.channel = p_channel
    RETURNING m.top_offset INTO v_offset;

    -- Insert the stream row. created_at defaults to NOW() so partition
    -- routing picks today's partition automatically.
    INSERT INTO __STREAM_TABLE__ (
        channel, channel_offset, epoch, kind, data, tags,
        client_id, user_id, conn_info, chan_info, key, prev_data, shard_id
    ) VALUES (
        p_channel, v_offset, v_epoch, 0, p_data, p_tags,
        p_client_id, p_user_id, p_conn_info, p_chan_info, p_key, v_prev_data, v_shard_id
    ) RETURNING id INTO v_id;

    -- Save idempotency result for replay.
    IF p_idempotency_key IS NOT NULL THEN
        INSERT INTO __PREFIX__idempotency (channel, idempotency_key, result_offset, result_id, expires_at)
        VALUES (p_channel, p_idempotency_key, v_offset, v_id, NOW() + COALESCE(p_idempotency_ttl, INTERVAL '5 minutes'))
        ON CONFLICT (channel, idempotency_key) DO NOTHING;
    END IF;

    -- Wake up notification listeners.
    PERFORM pg_notify('__PREFIX__notify', '');

    RETURN QUERY SELECT v_id, v_offset, v_epoch, FALSE, NULL::TEXT;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__publish_strict: identical to publish but raises on suppression.
-- Used by callers that want hard errors instead of suppressed-result handling.
CREATE OR REPLACE FUNCTION __PREFIX__publish_strict(
    p_channel TEXT,
    p_data __DATA_TYPE__,
    p_tags JSONB DEFAULT NULL,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_key TEXT DEFAULT NULL,
    p_history_ttl INTERVAL DEFAULT NULL,
    p_history_size INTEGER DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_version BIGINT DEFAULT NULL,
    p_version_epoch TEXT DEFAULT NULL,
    p_use_delta BOOLEAN DEFAULT FALSE,
    p_num_shards INTEGER DEFAULT NULL
) RETURNS TABLE(
    out_result_id BIGINT,
    out_channel_offset BIGINT,
    out_epoch TEXT
) AS $$
DECLARE
    v_result RECORD;
BEGIN
    SELECT * INTO v_result FROM __PREFIX__publish(
        p_channel, p_data, p_tags, p_client_id, p_user_id, p_conn_info, p_chan_info,
        p_key, p_history_ttl, p_history_size, p_meta_ttl,
        p_idempotency_key, p_idempotency_ttl,
        p_version, p_version_epoch, p_use_delta, p_num_shards
    );
    IF v_result.out_suppressed THEN
        RAISE EXCEPTION 'publish suppressed: %', v_result.out_suppress_reason;
    END IF;
    RETURN QUERY SELECT v_result.out_result_id, v_result.out_channel_offset, v_result.out_epoch;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__publish_join: insert a join (kind=1) row, no offset increment,
-- no meta update. Acquires the shard lock for outbox id-ordering correctness.
CREATE OR REPLACE FUNCTION __PREFIX__publish_join(
    p_channel TEXT,
    p_client_id TEXT,
    p_user_id TEXT,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_num_shards INTEGER DEFAULT NULL
) RETURNS BIGINT AS $$
DECLARE
    v_id BIGINT;
    v_shard_id INTEGER;
BEGIN
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;
    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    -- Acquire shard lock so this insert serializes with publications on the
    -- same shard. Without this, the join's id could be committed before an
    -- in-progress publication's id, causing the outbox worker to advance
    -- past the in-progress row.
    PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    INSERT INTO __STREAM_TABLE__ (
        channel, channel_offset, kind, client_id, user_id, conn_info, chan_info, shard_id
    ) VALUES (
        p_channel, 0, 1, p_client_id, p_user_id, p_conn_info, p_chan_info, v_shard_id
    ) RETURNING id INTO v_id;

    PERFORM pg_notify('__PREFIX__notify', '');
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__publish_leave: insert a leave (kind=2) row. Same shape as publish_join.
CREATE OR REPLACE FUNCTION __PREFIX__publish_leave(
    p_channel TEXT,
    p_client_id TEXT,
    p_user_id TEXT,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_num_shards INTEGER DEFAULT NULL
) RETURNS BIGINT AS $$
DECLARE
    v_id BIGINT;
    v_shard_id INTEGER;
BEGIN
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;
    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    INSERT INTO __STREAM_TABLE__ (
        channel, channel_offset, kind, client_id, user_id, conn_info, chan_info, shard_id
    ) VALUES (
        p_channel, 0, 2, p_client_id, p_user_id, p_conn_info, p_chan_info, v_shard_id
    ) RETURNING id INTO v_id;

    PERFORM pg_notify('__PREFIX__notify', '');
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__remove_history: wipes publications (kind=0) for a channel.
-- Acquires the shard lock so that any in-progress publish either commits
-- before the DELETE or runs after — no in-flight publication can survive
-- the wipe semantically.
CREATE OR REPLACE FUNCTION __PREFIX__remove_history(
    p_channel TEXT,
    p_num_shards INTEGER DEFAULT NULL
) RETURNS VOID AS $$
DECLARE
    v_shard_id INTEGER;
BEGIN
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;
    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    DELETE FROM __STREAM_TABLE__ WHERE channel = p_channel AND kind = 0;
    -- Note: meta epoch is NOT reset — matches Redis broker behavior. Joins
    -- and leaves (kind=1/2) are also kept and age out via partition retention.
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__top_position: Get current stream top position (top_offset + epoch).
-- Lightweight read for the external-state pattern: the application calls this
-- inside the same transaction as reading its own data, capturing the stream
-- position that covers any mutations during/after the state read. The client
-- SDK then subscribes with this position and recovers any publications that
-- arrived between the read and the subscribe.
CREATE OR REPLACE FUNCTION __PREFIX__top_position(
    p_channel TEXT
) RETURNS TABLE(
    out_top_offset BIGINT,
    out_epoch TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT COALESCE(m.top_offset, 0::BIGINT), COALESCE(m.epoch, ''::TEXT)
    FROM (SELECT 1) AS _dummy
    LEFT JOIN __PREFIX__meta m ON m.channel = p_channel;
END;
$$ LANGUAGE plpgsql;
