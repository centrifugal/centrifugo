// Package pgoutbox provides reusable outbox-pattern machinery for
// PostgreSQL-backed brokers.
//
// The package is designed to be shared between the PG map broker
// (centrifugo/internal/pgmapbroker) and a future PG stream broker. It
// contains only the generic parts of the outbox pattern: polling loop,
// advisory-lock coordination for one-poller-per-shard, LISTEN/NOTIFY
// wakeup, and daily table partition maintenance. All SQL and row→publication
// conversion stays in the calling broker.
//
// # Outbox pattern overview
//
// The caller maintains a stream table backed by a BIGSERIAL id column and a
// per-shard lock that serializes inserts within a shard. This combination
// guarantees that committed ids within a shard form a contiguous sequence
// — no gaps — which makes cursor-based polling safe with a simple
// `id > cursor` filter.
//
// Workers poll the stream table in batches and deliver publications to
// subscribers. Two worker shapes are provided:
//
//   - Worker — plain polling, one goroutine per shard on every node.
//     Used when the caller is the sole consumer (no cross-node fan-out).
//
//   - LockWorker — advisory-lock coordinated, one goroutine per shard with
//     a session-scoped lock held on the primary pool. Only one node per
//     shard actually polls; others wait to acquire the lock. Used when the
//     caller fans out publications via an inner Broker (Redis/NATS) and
//     must avoid duplicate delivery.
//
// # Concurrency model
//
// One goroutine per worker. Workers are goroutine-safe by construction
// because each owns its own cursor state and never shares memory with
// other workers. Error and info callbacks may be invoked from multiple
// workers concurrently and must be safe for concurrent use by the caller.
//
// # Lifecycle
//
// Each worker takes (ctx, closeCh) in Run(). Shutdown is signalled by
// cancelling ctx AND/OR closing closeCh. The caller is expected to cancel
// ctx first (to interrupt in-flight pgx calls and WaitForNotification)
// and then close closeCh (to signal the poll loop between batches).
//
// closeCh is a <-chan struct{} and is observed via a select case. It must
// be closed, never sent on. NotifyCh is a different story — see below.
//
// # NotifyCh invariants
//
// Worker's NotifyCh is a <-chan struct{} used as a wakeup signal from a
// LISTEN/NOTIFY loop. It must be a send-only channel from the poll loop's
// perspective. The caller owns the channel and its sender side.
//
// NotifyCh must NEVER be closed. A closed channel fires immediately in
// select statements and would cause the worker to spin. A nil channel is
// fine — a nil case in select is runtime-inert (blocks forever, never
// fires), so passing nil is equivalent to "no wakeup signal, rely on
// PollInterval".
//
// LockWorker does not take a NotifyCh because the current advisory-lock
// variant does not use LISTEN/NOTIFY. If this changes, the type should
// gain an explicit field rather than implicitly sharing Worker's.
//
// # Partitioning
//
// Partitioner manages daily time-based partition maintenance for tables
// declared as PARTITION BY RANGE (created_at). It does NOT create the
// parent partitioned table itself — the parent's column list is schema-
// specific and must stay with the calling broker. Partitioner only owns
// the ongoing maintenance: creating tomorrow's partition before it's
// needed, and dropping partitions older than retention.
//
// The partition naming convention is {ParentTable}_{YYYY}_{MM}_{DD}.
// Partitions that don't match this pattern are skipped by DropOldPartitions
// — they are assumed to be user-managed and not subject to automatic
// cleanup.
//
// # Dependencies
//
// This package imports only stdlib and github.com/jackc/pgx/v5/pgxpool.
// No centrifuge, no zerolog, no pgmapbroker. Logging is done via function-
// field callbacks (ErrorFn, InfoFn) provided by the caller.
package pgoutbox
