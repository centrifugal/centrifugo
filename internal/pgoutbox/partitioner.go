package pgoutbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Partitioner manages daily time-based partition maintenance for an
// existing PARTITION BY RANGE (created_at) parent table.
//
// It does NOT create the parent partitioned table itself — the parent's
// column list is schema-specific and must be created by the calling
// broker before any Partitioner methods are called. Partitioner only
// handles the ongoing maintenance:
//
//   - EnsureLookaheadPartitions creates today's partition and up to
//     LookaheadDays future daily partitions via CREATE TABLE IF NOT EXISTS
//     ... PARTITION OF. Safe to call repeatedly. Used both by the caller
//     at schema init time and by Run on a periodic ticker.
//
//   - DropOldPartitions lists child partitions via pg_inherits, parses
//     the date from the partition name suffix, and drops any partition
//     older than (now - RetentionDays). Errors on individual drops are
//     logged via ErrorFn rather than returned, so a failed drop does not
//     prevent the cleanup pass from attempting the remaining partitions.
//
//   - Run starts a ticker that calls both methods on each tick.
//
// Partition naming convention: {ParentTable}_{YYYY}_{MM}_{DD}. Partitions
// that don't match this convention are silently skipped by DropOldPartitions
// — they are assumed to be user-managed and are not subject to automatic
// cleanup.
type Partitioner struct {
	// Pool is the pgxpool used for all partition maintenance SQL.
	// Typically, the primary pool (DDL must target the primary).
	Pool *pgxpool.Pool

	// ParentTable is the unquoted name of the parent partitioned table
	// (e.g. "cf_map_stream").
	ParentTable string

	// CleanupInterval is the ticker interval for Run.
	CleanupInterval time.Duration

	// LookaheadDays controls how many future daily partitions to pre-create.
	// 0 means only today's partition.
	LookaheadDays int

	// RetentionDays controls how old a partition must be before
	// DropOldPartitions removes it. Partitions dated strictly before
	// (now - RetentionDays) are dropped.
	RetentionDays int

	// ErrorFn is called for non-ctx errors that do not abort the pass
	// (e.g. per-partition drop failures, listing failures in the
	// cleanup path). Must be safe for concurrent use.
	ErrorFn func(msg string, err error)
}

// EnsureLookaheadPartitions creates today's daily partition and up to
// LookaheadDays future daily partitions as PARTITION OF ParentTable.
// The CREATE TABLE IF NOT EXISTS form makes this safe to call repeatedly.
//
// Boundaries are written as fully-qualified UTC timestamps (e.g.
// '2026-04-23 00:00:00+00'). Using a bare DATE/date-text literal would
// bind the boundary to the PostgreSQL session's timezone: a server
// configured for Asia/Nicosia would treat '2026-04-23' as
// 2026-04-22 21:00 UTC, producing partitions that are offset from the
// UTC calendar day and don't cover rows inserted in the last hours of a
// UTC day.
func (p *Partitioner) EnsureLookaheadPartitions(ctx context.Context) error {
	now := time.Now().UTC()
	for d := 0; d <= p.LookaheadDays; d++ {
		day := now.AddDate(0, 0, d)
		nextDay := day.AddDate(0, 0, 1)
		partName := fmt.Sprintf("%s_%s", p.ParentTable, day.Format("2006_01_02"))
		_, err := p.Pool.Exec(ctx, fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
			partName, p.ParentTable,
			day.Format("2006-01-02 00:00:00+00"),
			nextDay.Format("2006-01-02 00:00:00+00"),
		))
		if err != nil {
			return fmt.Errorf("create partition %s: %w", partName, err)
		}
	}
	return nil
}

// DropOldPartitions lists child partitions of ParentTable via pg_inherits,
// parses the date from each partition name, and drops partitions dated
// before (now - RetentionDays). Errors on individual drops (or the list
// query itself) are logged via ErrorFn rather than returned.
//
// When RetentionDays is 0 or negative, this is a no-op: the caller wants
// "lookahead only, never drop" semantics. Without this guard, RetentionDays=0
// would compute cutoff = now, dropping every partition dated strictly before
// today (i.e. yesterday and older) — equivalent to a 1-day retention, not
// "no retention".
func (p *Partitioner) DropOldPartitions(ctx context.Context) {
	if p.RetentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -p.RetentionDays)

	rows, err := p.Pool.Query(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class pp ON pp.oid = i.inhparent
		WHERE pp.relname = $1
	`, p.ParentTable)
	if err != nil {
		p.ErrorFn("error listing partitions for cleanup", err)
		return
	}
	defer rows.Close()

	var partNames []string
	for rows.Next() {
		var partName string
		if err := rows.Scan(&partName); err != nil {
			continue
		}
		partNames = append(partNames, partName)
	}
	// Close rows before issuing further queries on the same pool.
	rows.Close()

	for _, partName := range partNames {
		partDate, ok := parsePartitionDate(partName, p.ParentTable)
		if !ok {
			continue
		}
		if partDate.Before(cutoff) {
			if _, err := p.Pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", partName)); err != nil {
				p.ErrorFn("error dropping old partition "+partName, err)
			}
		}
	}
}

// Run starts a ticker that calls EnsureLookaheadPartitions and
// DropOldPartitions on each CleanupInterval tick. Returns when ctx is
// cancelled or closeCh is closed.
func (p *Partitioner) Run(ctx context.Context, closeCh <-chan struct{}) {
	ticker := time.NewTicker(p.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.EnsureLookaheadPartitions(ctx); err != nil {
				p.ErrorFn("error ensuring lookahead partitions", err)
			}
			p.DropOldPartitions(ctx)
		}
	}
}

// parsePartitionDate parses the date suffix from a child partition name
// of the form {ParentTable}_{YYYY}_{MM}_{DD}. Returns (time, true) on
// success; (zero, false) on any parse failure (too-short name, non-date
// suffix, invalid calendar date, etc.). Failures are silent — they
// indicate a partition that doesn't follow the expected naming convention
// and should be left alone by automatic cleanup.
//
// The parentTable argument is currently unused by the parser (the date
// suffix is interpreted positionally as the last three underscore-separated
// components). It is retained in the signature so future refinements can
// validate the prefix without a breaking change.
func parsePartitionDate(partName, _ string) (time.Time, bool) {
	parts := strings.Split(partName, "_")
	if len(parts) < 3 {
		return time.Time{}, false
	}
	dateStr := strings.Join(parts[len(parts)-3:], "-")
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
