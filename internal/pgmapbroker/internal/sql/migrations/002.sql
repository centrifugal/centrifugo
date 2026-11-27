-- Schema version 2: ensure_meta, and publish drops the TTL it sets with the
-- first publication. Don't edit: this is version 2 as released, later changes
-- of these functions go to migrations of their own and to schema.sql.

CREATE OR REPLACE FUNCTION __PREFIX__publish(
    p_channel TEXT,
    p_key TEXT,
    p_data __DATA_TYPE__,
    p_tags JSONB DEFAULT NULL,
    p_key_ttl INTERVAL DEFAULT NULL,
    p_meta_ttl INTERVAL DEFAULT NULL,
    p_key_mode TEXT DEFAULT NULL,
    p_expected_offset BIGINT DEFAULT NULL,
    p_expected_epoch TEXT DEFAULT NULL,
    p_key_version BIGINT DEFAULT NULL,
    p_key_version_epoch TEXT DEFAULT NULL,
    p_idempotency_key TEXT DEFAULT NULL,
    p_idempotency_ttl INTERVAL DEFAULT NULL,
    p_use_delta BOOLEAN DEFAULT FALSE,
    p_client_id TEXT DEFAULT NULL,
    p_user_id TEXT DEFAULT NULL,
    p_conn_info __DATA_TYPE__ DEFAULT NULL,
    p_chan_info __DATA_TYPE__ DEFAULT NULL,
    p_refresh_ttl_on_suppress BOOLEAN DEFAULT FALSE,
    p_num_shards INTEGER DEFAULT NULL
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
    v_idempotent_epoch TEXT;
    v_shard_id INTEGER;
    v_meta_inserted BOOLEAN;
BEGIN
    -- Validate: meta_ttl must outlive key_ttl. When keys are permanent
    -- (p_key_ttl IS NULL), meta must also be permanent (p_meta_ttl IS NULL).
    IF p_meta_ttl IS NOT NULL AND p_key_ttl IS NULL THEN
        RAISE EXCEPTION 'meta_ttl must be NULL (permanent) when key_ttl is NULL (permanent keys)';
    END IF;
    IF p_meta_ttl IS NOT NULL AND p_key_ttl IS NOT NULL AND p_meta_ttl < p_key_ttl THEN
        RAISE EXCEPTION 'meta_ttl must be >= key_ttl (metadata must outlive keys)';
    END IF;

    -- Auto-derive num_shards from shard_lock table when not provided.
    IF p_num_shards IS NULL THEN
        SELECT COUNT(*)::INTEGER INTO p_num_shards FROM __PREFIX__shard_lock;
    END IF;
    IF p_num_shards <= 0 THEN
        RAISE EXCEPTION 'shard_lock table is empty — run EnsureSchema first';
    END IF;

    -- Calculate shard_id from channel hash
    v_shard_id := abs(hashtext(p_channel)::bigint) % p_num_shards;

    -- 0. Per-shard serialization lock (lock order: shard → meta → state)
    PERFORM 1 FROM __PREFIX__shard_lock WHERE shard_id = v_shard_id FOR UPDATE;

    -- 1. Get or create stream metadata. Atomic UPSERT with no-op write on
    -- conflict acquires the row lock as part of the same statement, avoiding
    -- the race where DO NOTHING + SELECT FOR UPDATE could return zero rows
    -- if a concurrent cleanup deleted the meta between the two statements.
    -- Qualified column references (m.top_offset, m.epoch) disambiguate from
    -- the function's OUT parameter named "epoch".
    --
    -- (xmax = 0) distinguishes a fresh INSERT from a CONFLICT-DO-UPDATE: when
    -- the meta row was just created (e.g. cleanupEntries dropped the prior
    -- one), we wipe the channel's state below — otherwise zombie state rows
    -- from the dead epoch would be returned by ReadState/Stats with stale
    -- key_offset values until expire_keys catches up. Mirrors the Redis
    -- broker's `if top_offset == 1 then del stream_key` safety net.
    INSERT INTO __PREFIX__meta AS m (channel, top_offset, epoch, updated_at)
    VALUES (p_channel, 0, substr(md5(random()::text || random()::text), 1, 8), NOW())
    ON CONFLICT (channel) DO UPDATE
        SET updated_at = m.updated_at
    RETURNING m.top_offset, m.epoch, (xmax = 0) INTO v_offset, v_epoch, v_meta_inserted;

    IF v_meta_inserted THEN
        DELETE FROM __PREFIX__state WHERE channel = p_channel;
    END IF;

    -- 2. Check idempotency. Return the epoch the publication was committed
    -- under (stored in the idempotency row), NOT the current meta epoch —
    -- those can differ if meta was TTL-evicted and recreated between the
    -- original publish and this replay.
    IF p_idempotency_key IS NOT NULL THEN
        SELECT result_offset, result_epoch INTO v_current_offset, v_idempotent_epoch
        FROM __PREFIX__idempotency
        WHERE channel = p_channel AND idempotency_key = p_idempotency_key
          AND expires_at > NOW();
        IF FOUND THEN
            RETURN QUERY SELECT NULL::BIGINT, v_current_offset, v_idempotent_epoch, TRUE,
                'idempotency'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
    END IF;

    -- Canonical check order across brokers: Idempotency → Version → KeyMode → CAS.
    -- Dedup checks (idempotency, version) drop duplicates first; constraint checks
    -- (KeyMode, CAS) report meaningful intent failures last.

    -- 3. Per-key version check (dedup)
    IF p_key_version IS NOT NULL AND p_key IS NOT NULL AND p_key != '' THEN
        SELECT sn.key_version, sn.key_version_epoch
        INTO v_current_version, v_current_version_epoch
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
        IF FOUND AND v_current_version IS NOT NULL AND v_current_version > 0 THEN
            -- Empty p_key_version_epoch matches any stored epoch (matches the
            -- stream broker contract: "I don't care about epoch boundaries,
            -- just compare versions"). A non-empty value that differs resets
            -- the comparison so a new epoch isn't suppressed by a stale
            -- higher version under a previous epoch.
            IF (p_key_version_epoch IS NULL OR p_key_version_epoch = '' OR p_key_version_epoch = v_current_version_epoch)
               AND p_key_version <= v_current_version THEN
                RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'version'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
                RETURN;
            END IF;
        END IF;
    END IF;

    -- 4. KeyMode check — physical existence in state table, matching Redis HEXISTS.
    -- Zombie entries (TTL passed but cleanup worker hasn't emitted LEAVE yet) still
    -- count as existing: from the channel's perspective, the key is still "live" until
    -- the removal publication is written to the stream.
    IF p_key_mode IS NOT NULL THEN
        SELECT EXISTS(SELECT 1 FROM __PREFIX__state WHERE channel = p_channel AND key = p_key) INTO v_exists;
        IF p_key_mode = 'if_new' AND v_exists THEN
            IF p_refresh_ttl_on_suppress AND p_key_ttl IS NOT NULL THEN
                UPDATE __PREFIX__state SET expires_at = NOW() + p_key_ttl, updated_at = NOW()
                WHERE channel = p_channel AND key = p_key;
                -- Keepalive must extend meta TTL too, otherwise meta expires while
                -- keys are being refreshed and the next publish creates a new epoch
                -- (channel "blinks" — clients see ErrorUnrecoverablePosition).
                IF p_meta_ttl IS NOT NULL THEN
                    UPDATE __PREFIX__meta SET expires_at = NOW() + p_meta_ttl, updated_at = NOW()
                    WHERE channel = p_channel;
                END IF;
            END IF;
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'key_exists'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
        IF p_key_mode = 'if_exists' AND NOT v_exists THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE, 'key_not_found'::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
            RETURN;
        END IF;
    END IF;

    -- 5. CAS check (ExpectedPosition). Compare BOTH offset and epoch (when
    -- the caller supplies an expected epoch) so that a CAS using a position
    -- from a dead epoch — e.g. after a Clear+republish that flips the channel
    -- epoch but produces matching offsets — is correctly rejected. Memory and
    -- Redis brokers compare both; PostgreSQL must match for cross-broker
    -- consistency.
    IF p_expected_offset IS NOT NULL THEN
        SELECT key_offset, sn.data INTO v_current_offset, v_current_data
        FROM __PREFIX__state sn WHERE sn.channel = p_channel AND sn.key = p_key;
        IF NOT FOUND
           OR v_current_offset != p_expected_offset
           OR (p_expected_epoch IS NOT NULL AND v_epoch != p_expected_epoch) THEN
            RETURN QUERY SELECT NULL::BIGINT, v_offset, v_epoch, TRUE,
                'position_mismatch'::TEXT, v_current_data, v_current_offset;
            RETURN;
        END IF;
    END IF;

    -- 6. All checks passed - increment offset. Without a meta TTL the first
    -- publication drops the TTL set by ensure_meta, as if it created the row.
    UPDATE __PREFIX__meta SET
        top_offset = top_offset + 1,
        expires_at = CASE WHEN p_meta_ttl IS NOT NULL THEN NOW() + p_meta_ttl
                          WHEN top_offset = 0 THEN NULL
                          ELSE expires_at END,
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
        channel, key, data, tags, client_id, user_id, conn_info, chan_info,
        key_version, key_version_epoch, key_offset, expires_at, updated_at
    ) VALUES (
        p_channel, p_key, p_data, p_tags, p_client_id, p_user_id, p_conn_info, p_chan_info,
        p_key_version, p_key_version_epoch, v_offset,
        CASE WHEN p_key_ttl IS NOT NULL THEN NOW() + p_key_ttl ELSE NULL END, NOW()
    )
    ON CONFLICT (channel, key) DO UPDATE SET
        data = EXCLUDED.data, tags = EXCLUDED.tags,
        client_id = EXCLUDED.client_id, user_id = EXCLUDED.user_id,
        conn_info = EXCLUDED.conn_info, chan_info = EXCLUDED.chan_info,
        -- Preserve stored version when caller publishes without one (matches Redis).
        -- Overwriting with NULL would erase dedup protection against late-arriving
        -- older versions from a concurrent producer.
        key_version = COALESCE(EXCLUDED.key_version, __PREFIX__state.key_version),
        key_version_epoch = COALESCE(EXCLUDED.key_version_epoch, __PREFIX__state.key_version_epoch),
        key_offset = EXCLUDED.key_offset, expires_at = EXCLUDED.expires_at, updated_at = NOW();

    -- 8. Insert into stream (include epoch, shard_id, and prev_data for delta)
    INSERT INTO __PREFIX__stream (
        channel, channel_offset, epoch, key, data, tags, client_id, user_id, conn_info, chan_info, prev_data, shard_id
    ) VALUES (
        p_channel, v_offset, v_epoch, p_key, p_data, p_tags, p_client_id, p_user_id, p_conn_info, p_chan_info, v_previous_data,
        v_shard_id
    ) RETURNING __PREFIX__stream.id INTO v_id;

    -- 9. Save idempotency key. Persist the epoch alongside the offset so
    -- replays return the (offset, epoch) tuple this publish committed under,
    -- not whatever epoch happens to live in meta at replay time.
    IF p_idempotency_key IS NOT NULL THEN
        INSERT INTO __PREFIX__idempotency (channel, idempotency_key, result_offset, result_epoch, result_id, expires_at)
        VALUES (p_channel, p_idempotency_key, v_offset, v_epoch, v_id, NOW() + COALESCE(p_idempotency_ttl, INTERVAL '5 minutes'))
        ON CONFLICT DO NOTHING;
    END IF;

    -- 10. Notify outbox workers
    PERFORM pg_notify('__PREFIX__stream_notify', '');

    RETURN QUERY SELECT v_id, v_offset, v_epoch, FALSE, NULL::TEXT, NULL::__DATA_TYPE__, NULL::BIGINT;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION __PREFIX__ensure_meta(
    p_channel TEXT,
    p_ttl INTERVAL DEFAULT NULL
) RETURNS VOID AS $$
DECLARE
    v_inserted BOOLEAN;
BEGIN
    INSERT INTO __PREFIX__meta AS m (channel, epoch, expires_at, updated_at)
    VALUES (p_channel, substr(md5(random()::text || random()::text), 1, 8), NOW() + p_ttl, NOW())
    ON CONFLICT (channel) DO UPDATE
        SET expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at
        WHERE m.top_offset = 0 AND m.expires_at < EXCLUDED.expires_at
    RETURNING (m.xmax = 0) INTO v_inserted;

    IF v_inserted THEN
        DELETE FROM __PREFIX__state WHERE channel = p_channel;
        PERFORM set_config('synchronous_commit', 'off', true);
    END IF;
END;
$$ LANGUAGE plpgsql;
