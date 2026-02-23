-- ============================================================================
-- PostgreSQL MapBroker Schema and Functions (template).
-- Auto-created by EnsureSchema(). All statements are idempotent.
-- Placeholders replaced by `make pg-schemas`:
--   __DATA_TYPE__ → JSONB or BYTEA
--   __PREFIX__    → cf_map_ or cf_binary_map_
-- Do NOT edit the generated files (schema_jsonb.sql, schema_binary.sql).
-- ============================================================================

-- Stream Table (Change History + Fan-out)
CREATE TABLE IF NOT EXISTS __PREFIX__stream (
    id              BIGSERIAL PRIMARY KEY,
    channel         TEXT NOT NULL,
    channel_offset  BIGINT NOT NULL,
    epoch           TEXT NOT NULL DEFAULT '',
    key             TEXT NOT NULL,
    data            __DATA_TYPE__,
    tags            JSONB,
    client_id       TEXT,
    user_id         TEXT,
    conn_info       __DATA_TYPE__,
    chan_info        __DATA_TYPE__,
    subscribed_at   TIMESTAMPTZ,
    removed         BOOLEAN DEFAULT FALSE,
    score           BIGINT,
    previous_data   __DATA_TYPE__,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    shard_id        SMALLINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS __PREFIX__stream_channel_offset_idx ON __PREFIX__stream (channel, channel_offset);
CREATE INDEX IF NOT EXISTS __PREFIX__stream_channel_id_idx ON __PREFIX__stream (channel, id DESC);
CREATE INDEX IF NOT EXISTS __PREFIX__stream_shard_id_idx ON __PREFIX__stream (shard_id, id);
CREATE INDEX IF NOT EXISTS __PREFIX__stream_created_at_idx ON __PREFIX__stream (created_at);

-- ============================================================================
-- Snapshot Table (Current State)
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__state (
    channel             TEXT NOT NULL,
    key                 TEXT NOT NULL,
    data                __DATA_TYPE__,
    tags                JSONB,
    client_id           TEXT,
    user_id             TEXT,
    conn_info           __DATA_TYPE__,
    chan_info            __DATA_TYPE__,
    subscribed_at       TIMESTAMPTZ,
    score               BIGINT,
    key_version         BIGINT DEFAULT 0,
    key_version_epoch   TEXT,
    key_offset          BIGINT NOT NULL,
    expires_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (channel, key)
);

CREATE INDEX IF NOT EXISTS __PREFIX__state_ordered_idx
    ON __PREFIX__state (channel, score DESC, key)
    WHERE score IS NOT NULL;
CREATE INDEX IF NOT EXISTS __PREFIX__state_expires_idx
    ON __PREFIX__state (expires_at)
    WHERE expires_at IS NOT NULL;

-- ============================================================================
-- Stream Metadata Table
-- ============================================================================

CREATE TABLE IF NOT EXISTS __PREFIX__meta (
    channel         TEXT PRIMARY KEY,
    top_offset      BIGINT NOT NULL DEFAULT 0,
    epoch           TEXT NOT NULL DEFAULT '',
    version         BIGINT DEFAULT 0,
    version_epoch   TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS __PREFIX__meta_expires_idx
    ON __PREFIX__meta (expires_at)
    WHERE expires_at IS NOT NULL;

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

CREATE INDEX IF NOT EXISTS __PREFIX__idempotency_expires_idx ON __PREFIX__idempotency (expires_at);

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

CREATE OR REPLACE FUNCTION __PREFIX__publish(
    p_channel TEXT,
    p_key TEXT,
    p_data __DATA_TYPE__,
    p_tags JSONB DEFAULT NULL,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_subscribed_at TIMESTAMPTZ DEFAULT NULL,
    p_key_mode TEXT DEFAULT NULL,
    p_key_ttl INTERVAL DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_expected_offset BIGINT DEFAULT NULL,
    p_score BIGINT DEFAULT NULL,
    p_version BIGINT DEFAULT NULL,
    p_version_epoch TEXT DEFAULT NULL,
    p_key_version BIGINT DEFAULT NULL,
    p_key_version_epoch TEXT DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_refresh_ttl_on_suppress BOOLEAN DEFAULT FALSE,
    p_use_delta BOOLEAN DEFAULT FALSE,
    p_num_shards INTEGER DEFAULT NULL,
    p_stream_data __DATA_TYPE__ DEFAULT NULL,
    p_skip_shard_lock BOOLEAN DEFAULT FALSE
) RETURNS TABLE(
    result_id BIGINT,
    channel_offset BIGINT,
    epoch TEXT,
    suppressed BOOLEAN,
    suppress_reason TEXT,
    current_data __DATA_TYPE__,
    current_offset BIGINT
) AS $$
DECLARE
    v_offset BIGINT;
    v_id BIGINT;
    v_epoch TEXT;
    v_exists BOOLEAN;
    v_current_offset BIGINT;
    v_current_data __DATA_TYPE__;
    v_previous_data __DATA_TYPE__;
    v_current_version BIGINT;
    v_current_version_epoch TEXT;
    v_shard_id INTEGER;
BEGIN
    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;

    -- Calculate shard_id from channel hash
    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    -- 0. Per-shard serialization lock (lock order: shard → meta → state)
    IF NOT p_skip_shard_lock THEN
        PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;
    END IF;

    -- 1. Get or create stream metadata
    INSERT INTO __PREFIX__meta (channel, top_offset, epoch, updated_at)
    VALUES (p_channel, 0, substr(md5(random()::text || random()::text), 1, 8), NOW())
    ON CONFLICT (channel) DO NOTHING;

    SELECT top_offset, m.epoch
    INTO v_offset, v_epoch
    FROM __PREFIX__meta m WHERE m.channel = p_channel FOR UPDATE;

    -- 2. Check idempotency
    IF p_idempotency_key IS NOT NULL THEN
        SELECT result_offset INTO v_current_offset
        FROM __PREFIX__idempotency
        WHERE channel = p_channel AND idempotency_key = p_idempotency_key
          AND expires_at > NOW();
        IF FOUND THEN
            RETURN QUERY SELECT NULL::BIGINT, v_current_offset, v_epoch, TRUE,
                'idempotency'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
    END IF;

    -- 3. CAS check (ExpectedPosition)
    IF p_expected_offset IS NOT NULL THEN
        SELECT key_offset, sn.data INTO v_current_offset, v_current_data
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
        IF NOT FOUND OR v_current_offset != p_expected_offset THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE,
                'position_mismatch'::TEXT, v_current_data, v_current_offset;
            RETURN;
        END IF;
    END IF;

    -- 4. KeyMode check (exclude expired entries — they are logically deleted)
    IF p_key_mode IS NOT NULL THEN
        SELECT EXISTS(SELECT 1 FROM __PREFIX__state WHERE channel = p_channel AND key = p_key AND (expires_at IS NULL OR expires_at > NOW())) INTO v_exists;
        IF p_key_mode = 'if_new' AND v_exists THEN
            IF p_refresh_ttl_on_suppress AND p_key_ttl IS NOT NULL THEN
                UPDATE __PREFIX__state SET expires_at = NOW() + p_key_ttl, updated_at = NOW()
                WHERE channel = p_channel AND key = p_key;
            END IF;
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'key_exists'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
        IF p_key_mode = 'if_exists' AND NOT v_exists THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'key_not_found'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
    END IF;

    -- 5. Per-key version check
    IF p_key_version IS NOT NULL AND p_key IS NOT NULL AND p_key != '' THEN
        SELECT sn.key_version, sn.key_version_epoch
        INTO v_current_version, v_current_version_epoch
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
        IF FOUND AND v_current_version IS NOT NULL AND v_current_version > 0 THEN
            IF (p_key_version_epoch IS NULL OR p_key_version_epoch = v_current_version_epoch)
               AND p_key_version <= v_current_version THEN
                RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'version'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
                RETURN;
            END IF;
        END IF;
    END IF;

    -- 6. All checks passed - increment offset
    UPDATE __PREFIX__meta SET
        top_offset = top_offset + 1,
        expires_at = COALESCE(CASE WHEN p_meta_ttl IS NOT NULL THEN NOW() + p_meta_ttl ELSE NULL END, expires_at),
        updated_at = NOW()
    WHERE channel = p_channel
    RETURNING top_offset INTO v_offset;

    -- 6b. Fetch previous data for key-based delta (before UPSERT overwrites it)
    IF p_use_delta AND p_key IS NOT NULL AND p_key != '' THEN
        SELECT sn.data INTO v_previous_data
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
    END IF;

    -- 7. Update snapshot
    INSERT INTO __PREFIX__state (
        channel, key, data, tags, client_id, user_id, conn_info, chan_info, subscribed_at,
        score, key_version, key_version_epoch, key_offset, expires_at, updated_at
    ) VALUES (
        p_channel, p_key, p_data, p_tags, p_client_id, p_user_id, p_conn_info, p_chan_info, p_subscribed_at,
        p_score, p_key_version, p_key_version_epoch, v_offset,
        CASE WHEN p_key_ttl IS NOT NULL THEN NOW() + p_key_ttl ELSE NULL END, NOW()
    )
    ON CONFLICT (channel, key) DO UPDATE SET
        data = EXCLUDED.data, tags = EXCLUDED.tags,
        client_id = EXCLUDED.client_id, user_id = EXCLUDED.user_id,
        conn_info = EXCLUDED.conn_info, chan_info = EXCLUDED.chan_info, subscribed_at = EXCLUDED.subscribed_at,
        score = EXCLUDED.score, key_version = EXCLUDED.key_version, key_version_epoch = EXCLUDED.key_version_epoch,
        key_offset = EXCLUDED.key_offset, expires_at = EXCLUDED.expires_at, updated_at = NOW();

    -- 8. Insert into stream (include epoch, shard_id, and previous_data for delta)
    INSERT INTO __PREFIX__stream (
        channel, channel_offset, epoch, key, data, tags, client_id, user_id, conn_info, chan_info, subscribed_at, score, previous_data, shard_id
    ) VALUES (
        p_channel, v_offset, v_epoch, p_key, COALESCE(p_stream_data, p_data), p_tags, p_client_id, p_user_id, p_conn_info, p_chan_info, p_subscribed_at, p_score, v_previous_data,
        v_shard_id
    ) RETURNING __PREFIX__stream.id INTO v_id;

    -- 9. Save idempotency key
    IF p_idempotency_key IS NOT NULL THEN
        INSERT INTO __PREFIX__idempotency (channel, idempotency_key, result_offset, result_id, expires_at)
        VALUES (p_channel, p_idempotency_key, v_offset, v_id, NOW() + COALESCE(p_idempotency_ttl, INTERVAL '5 minutes'))
        ON CONFLICT DO NOTHING;
    END IF;

    -- 10. Notify outbox workers
    PERFORM pg_notify('__PREFIX__stream_notify', '');

    RETURN QUERY SELECT v_id, v_offset, v_epoch, FALSE, NULL::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__publish_strict: Auto-rollback on suppression
CREATE OR REPLACE FUNCTION __PREFIX__publish_strict(
    p_channel TEXT,
    p_key TEXT,
    p_data __DATA_TYPE__,
    p_tags JSONB DEFAULT NULL,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_subscribed_at TIMESTAMPTZ DEFAULT NULL,
    p_key_mode TEXT DEFAULT NULL,
    p_key_ttl INTERVAL DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_expected_offset BIGINT DEFAULT NULL,
    p_score BIGINT DEFAULT NULL,
    p_version BIGINT DEFAULT NULL,
    p_version_epoch TEXT DEFAULT NULL,
    p_key_version BIGINT DEFAULT NULL,
    p_key_version_epoch TEXT DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_refresh_ttl_on_suppress BOOLEAN DEFAULT FALSE,
    p_use_delta BOOLEAN DEFAULT FALSE,
    p_num_shards INTEGER DEFAULT NULL,
    p_stream_data __DATA_TYPE__ DEFAULT NULL,
    p_skip_shard_lock BOOLEAN DEFAULT FALSE
) RETURNS TABLE(
    result_id BIGINT,
    channel_offset BIGINT,
    epoch TEXT
) AS $$
DECLARE
    v_result RECORD;
BEGIN
    SELECT * INTO v_result
    FROM __PREFIX__publish(
        p_channel, p_key, p_data, p_tags,
        p_client_id, p_user_id, p_conn_info, p_chan_info, p_subscribed_at,
        p_key_mode, p_key_ttl, p_meta_ttl,
        p_expected_offset, p_score, p_version, p_version_epoch,
        p_key_version, p_key_version_epoch,
        p_idempotency_key, p_idempotency_ttl, p_refresh_ttl_on_suppress,
        p_use_delta, p_num_shards, p_stream_data, p_skip_shard_lock
    );

    IF v_result.suppressed THEN
        CASE v_result.suppress_reason
            WHEN 'key_exists' THEN
                RAISE EXCEPTION '__PREFIX__publish: key already exists: %.%', p_channel, p_key
                    USING ERRCODE = 'unique_violation';
            WHEN 'key_not_found' THEN
                RAISE EXCEPTION '__PREFIX__publish: key not found: %.%', p_channel, p_key
                    USING ERRCODE = 'no_data_found';
            WHEN 'position_mismatch' THEN
                RAISE EXCEPTION '__PREFIX__publish: CAS conflict on %.%', p_channel, p_key
                    USING ERRCODE = 'serialization_failure',
                          DETAIL = v_result.current_data::TEXT;
            WHEN 'version' THEN
                RAISE EXCEPTION '__PREFIX__publish: version conflict on %.%', p_channel, p_key
                    USING ERRCODE = 'serialization_failure';
            WHEN 'idempotency' THEN
                RAISE EXCEPTION '__PREFIX__publish: duplicate idempotency key: %.%', p_channel, p_key
                    USING ERRCODE = 'unique_violation';
            ELSE
                RAISE EXCEPTION '__PREFIX__publish: suppressed: %', v_result.suppress_reason
                    USING ERRCODE = 'raise_exception';
        END CASE;
    END IF;

    RETURN QUERY SELECT v_result.result_id, v_result.channel_offset, v_result.epoch;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__remove: Remove a key
CREATE OR REPLACE FUNCTION __PREFIX__remove(
    p_channel TEXT,
    p_key TEXT,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_num_shards INTEGER DEFAULT NULL,
    p_expected_offset BIGINT DEFAULT NULL,
    p_skip_shard_lock BOOLEAN DEFAULT FALSE
) RETURNS TABLE(
    result_id BIGINT,
    channel_offset BIGINT,
    epoch TEXT,
    suppressed BOOLEAN,
    suppress_reason TEXT,
    current_data __DATA_TYPE__,
    current_offset BIGINT
) AS $$
DECLARE
    v_offset BIGINT;
    v_id BIGINT;
    v_epoch TEXT;
    v_exists BOOLEAN;
    v_shard_id INTEGER;
    v_current_offset BIGINT;
    v_current_data __DATA_TYPE__;
BEGIN
    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;

    -- Calculate shard_id from channel hash
    v_shard_id := abs(hashtext(p_channel)) % p_num_shards;

    -- 0. Per-shard serialization lock (lock order: shard → meta → state)
    IF NOT p_skip_shard_lock THEN
        PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;
    END IF;

    -- 1. Get stream metadata
    SELECT top_offset, m.epoch INTO v_offset, v_epoch
    FROM __PREFIX__meta m WHERE m.channel = p_channel;

    IF NOT FOUND THEN
        RETURN QUERY SELECT NULL::BIGINT, 0::BIGINT, ''::TEXT, TRUE, 'key_not_found'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
        RETURN;
    END IF;

    -- 2. Check idempotency
    IF p_idempotency_key IS NOT NULL THEN
        SELECT result_offset INTO v_offset
        FROM __PREFIX__idempotency
        WHERE channel = p_channel AND idempotency_key = p_idempotency_key
          AND expires_at > NOW();
        IF FOUND THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'idempotency'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
    END IF;

    -- 3. CAS check (ExpectedPosition)
    IF p_expected_offset IS NOT NULL THEN
        SELECT key_offset, sn.data INTO v_current_offset, v_current_data
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
        IF NOT FOUND OR v_current_offset != p_expected_offset THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE,
                'position_mismatch'::TEXT, v_current_data, v_current_offset;
            RETURN;
        END IF;
    END IF;

    -- 4. Check if key exists (exclude expired entries — they are logically deleted)
    SELECT EXISTS(SELECT 1 FROM __PREFIX__state WHERE channel = p_channel AND key = p_key AND (expires_at IS NULL OR expires_at > NOW())) INTO v_exists;
    IF NOT v_exists THEN
        RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'key_not_found'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
        RETURN;
    END IF;

    -- 5. Increment offset
    UPDATE __PREFIX__meta SET
        top_offset = top_offset + 1,
        expires_at = COALESCE(CASE WHEN p_meta_ttl IS NOT NULL THEN NOW() + p_meta_ttl ELSE NULL END, expires_at),
        updated_at = NOW()
    WHERE channel = p_channel
    RETURNING top_offset INTO v_offset;

    -- 6. Delete from snapshot
    DELETE FROM __PREFIX__state WHERE channel = p_channel AND key = p_key;

    -- 7. Insert removal into stream (include epoch and shard_id)
    INSERT INTO __PREFIX__stream (channel, channel_offset, epoch, key, removed, client_id, user_id, shard_id)
    VALUES (
        p_channel, v_offset, v_epoch, p_key, TRUE, p_client_id, p_user_id,
        v_shard_id
    ) RETURNING __PREFIX__stream.id INTO v_id;

    -- 8. Save idempotency key
    IF p_idempotency_key IS NOT NULL THEN
        INSERT INTO __PREFIX__idempotency (channel, idempotency_key, result_offset, result_id, expires_at)
        VALUES (p_channel, p_idempotency_key, v_offset, v_id, NOW() + COALESCE(p_idempotency_ttl, INTERVAL '5 minutes'))
        ON CONFLICT DO NOTHING;
    END IF;

    -- 9. Notify outbox workers
    PERFORM pg_notify('__PREFIX__stream_notify', '');

    RETURN QUERY SELECT v_id, v_offset, v_epoch, FALSE, NULL::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__remove_strict: Auto-rollback if key not found
CREATE OR REPLACE FUNCTION __PREFIX__remove_strict(
    p_channel TEXT,
    p_key TEXT,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_num_shards INTEGER DEFAULT NULL,
    p_expected_offset BIGINT DEFAULT NULL,
    p_skip_shard_lock BOOLEAN DEFAULT FALSE
) RETURNS TABLE(
    result_id BIGINT,
    channel_offset BIGINT,
    epoch TEXT
) AS $$
DECLARE
    v_result RECORD;
BEGIN
    SELECT * INTO v_result FROM __PREFIX__remove(
        p_channel, p_key, p_client_id, p_user_id,
        p_idempotency_key, p_idempotency_ttl, p_meta_ttl,
        p_num_shards, p_expected_offset, p_skip_shard_lock
    );

    IF v_result.suppressed THEN
        RAISE EXCEPTION '__PREFIX__remove: key not found: %.%', p_channel, p_key
            USING ERRCODE = 'no_data_found';
    END IF;

    RETURN QUERY SELECT v_result.result_id, v_result.channel_offset, v_result.epoch;
END;
$$ LANGUAGE plpgsql;

-- __PREFIX__expire_keys: Atomically expire keys that have passed their TTL.
-- Lock ordering: shard_lock → meta FOR UPDATE → state FOR UPDATE SKIP LOCKED.
-- This matches publish's lock order (shard → meta → state), preventing deadlocks.
CREATE OR REPLACE FUNCTION __PREFIX__expire_keys(
    p_batch_size INT DEFAULT 1000,
    p_num_shards INTEGER DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_channel TEXT DEFAULT NULL,
    p_skip_shard_lock BOOLEAN DEFAULT FALSE
) RETURNS TABLE(
    out_channel TEXT,
    out_key TEXT,
    out_offset BIGINT,
    out_epoch TEXT
) AS $$
DECLARE
    v_channel TEXT;
    v_keys TEXT[];
    v_count INT;
    v_base_offset BIGINT;
    v_epoch TEXT;
    v_shard_id INTEGER;
    v_processed INT := 0;
    v_notified BOOLEAN := FALSE;
BEGIN
    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;

    -- Process one channel at a time to maintain meta→state lock ordering.
    FOR v_channel IN
        SELECT DISTINCT s.channel FROM __PREFIX__state s
        WHERE s.expires_at IS NOT NULL AND s.expires_at <= NOW()
          AND (p_channel IS NULL OR s.channel = p_channel)
    LOOP
        -- 0. Calculate shard_id and acquire per-shard serialization lock
        --    (lock order: shard → meta → state).
        v_shard_id := abs(hashtext(v_channel)) % p_num_shards;
        IF NOT p_skip_shard_lock THEN
            PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;
        END IF;

        -- 1. Lock meta (same order as publish: shard → meta → state).
        SELECT top_offset, m.epoch INTO v_base_offset, v_epoch
        FROM __PREFIX__meta m WHERE m.channel = v_channel FOR UPDATE;

        IF NOT FOUND THEN
            DELETE FROM __PREFIX__state WHERE channel = v_channel
              AND expires_at IS NOT NULL AND expires_at <= NOW();
            CONTINUE;
        END IF;

        -- 2. Collect expired keys in one query (already under shard + meta locks).
        SELECT array_agg(key), count(*) INTO v_keys, v_count FROM (
            SELECT key FROM __PREFIX__state
            WHERE channel = v_channel
              AND expires_at IS NOT NULL AND expires_at <= NOW()
            LIMIT p_batch_size - v_processed
            FOR UPDATE SKIP LOCKED
        ) t;

        IF v_count IS NULL OR v_count = 0 THEN
            CONTINUE;
        END IF;

        -- Bump offset by count (1 UPDATE instead of N).
        UPDATE __PREFIX__meta SET
            top_offset = top_offset + v_count,
            expires_at = COALESCE(CASE WHEN p_meta_ttl IS NOT NULL THEN NOW() + p_meta_ttl ELSE NULL END, expires_at),
            updated_at = NOW()
        WHERE channel = v_channel
        RETURNING top_offset - v_count INTO v_base_offset;

        -- Batch delete (1 DELETE instead of N).
        DELETE FROM __PREFIX__state WHERE channel = v_channel AND key = ANY(v_keys);

        -- Batch insert removal events (1 INSERT instead of N).
        INSERT INTO __PREFIX__stream (channel, channel_offset, epoch, key, removed, shard_id)
        SELECT v_channel, v_base_offset + rn, v_epoch, k, TRUE, v_shard_id
        FROM unnest(v_keys) WITH ORDINALITY AS t(k, rn);

        -- Return results.
        RETURN QUERY
        SELECT v_channel, k, (v_base_offset + rn)::BIGINT, v_epoch
        FROM unnest(v_keys) WITH ORDINALITY AS t(k, rn);

        v_notified := TRUE;
        v_processed := v_processed + v_count;
        IF v_processed >= p_batch_size THEN
            PERFORM pg_notify('__PREFIX__stream_notify', '');
            RETURN;
        END IF;
    END LOOP;

    -- Notify outbox workers if any entries were expired.
    IF v_notified THEN
        PERFORM pg_notify('__PREFIX__stream_notify', '');
    END IF;
END;
$$ LANGUAGE plpgsql;
