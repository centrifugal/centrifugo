package pgmapbroker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgemptymeta"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reads which give a position create the epoch of an empty map with a stream.
// An empty epoch would match any later stream, and the map being empty again,
// so recovery could lose changes silently. See ensure_meta in schema.sql.

var errEmptyMeta = errors.New("meta of empty map is missing or due for refresh")

type emptyMeta struct {
	// hasEpoch is false for maps without a stream: nothing to recover from.
	hasEpoch bool
	// ttl of the meta, nil if it doesn't expire.
	ttl *string
	// keep is set with an initial TTL: subscribed empty maps are kept by
	// pgemptymeta.Keeper.
	keep bool
}

func (e *PostgresMapBroker) emptyMetaFor(chOpts centrifuge.MapChannelOptions) emptyMeta {
	if !chOpts.Mode.HasStream() {
		return emptyMeta{}
	}
	c := e.node.Config()
	initial := pgemptymeta.Initial(chOpts.MetaTTLInitial, c.HistoryMetaTTLInitial)
	em := emptyMeta{hasEpoch: true, keep: initial > 0}
	if ttl := pgemptymeta.TTL(chOpts.MetaTTL, initial, c.ClientChannelPositionCheckDelay); ttl > 0 {
		s := durationToIntervalString(ttl)
		em.ttl = &s
	}
	return em
}

// metaQuery selects what scanMeta reads. $1 is the channel, $2 emptyMeta.ttl.
func (e *PostgresMapBroker) metaQuery() string {
	return fmt.Sprintf(`
		SELECT top_offset, epoch,
		       COALESCE(top_offset = 0 AND expires_at < NOW() + $2::interval / 2, FALSE),
		       %s
		  FROM %s AS m
		 WHERE channel = $1
	`, pgemptymeta.RefreshInSQL, e.names.meta)
}

// scanMeta returns errEmptyMeta when ensure_meta has to run, pgx.ErrNoRows for
// a map which has neither meta nor epoch.
func (e *PostgresMapBroker) scanMeta(row pgx.Row, ch string, em emptyMeta) (centrifuge.StreamPosition, error) {
	var topOffset int64
	var epoch string
	var due bool
	var refreshIn *float64
	err := row.Scan(&topOffset, &epoch, &due, &refreshIn)
	if em.hasEpoch && (due || errors.Is(err, pgx.ErrNoRows)) {
		return centrifuge.StreamPosition{}, errEmptyMeta
	}
	if err != nil {
		return centrifuge.StreamPosition{}, err
	}
	if refreshIn != nil && em.keep {
		e.emptyMetaKeeper.Get().Watch(ch, *refreshIn, time.Now())
	}
	return centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}, nil
}

// withEmptyMeta runs read. If the meta needs a write the primary decides, as a
// replica may lag, and position and data are then read there too.
func withEmptyMeta[T any](
	ctx context.Context, e *PostgresMapBroker, pool *pgxpool.Pool, ch string, em emptyMeta,
	read func(pool *pgxpool.Pool) (T, error),
) (T, error) {
	res, err := read(pool)
	if errors.Is(err, errEmptyMeta) && pool != e.pool {
		res, err = read(e.pool)
	}
	if !errors.Is(err, errEmptyMeta) {
		return res, err
	}
	if _, err := e.pool.Exec(ctx, fmt.Sprintf(`SELECT %s($1, $2::interval)`, e.names.ensureMeta), ch, em.ttl); err != nil {
		return res, fmt.Errorf("postgres map broker: ensure meta: %w", err)
	}
	return read(e.pool)
}

func (e *PostgresMapBroker) isSubscribed(ch string) bool {
	_, ok := e.subscribed.Load(ch)
	return ok
}
