package pgoutbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise Worker's lifecycle, error handling, and cursor
// advancement using callback stubs. They do not require a running
// Postgres — Pool is passed as nil because the stub callbacks never
// dereference it.

const testRunTimeout = 2 * time.Second

// runWorkerAsync starts w.Run in a goroutine and returns a channel that
// closes when Run returns.
func runWorkerAsync(ctx context.Context, closeCh <-chan struct{}, w *Worker) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, closeCh)
	}()
	return done
}

func expectReturn(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(testRunTimeout):
		t.Fatal("worker did not return within timeout")
	}
}

func newTestWorker(process func(context.Context, *pgxpool.Pool, int64, []int) (int, int64, error)) *Worker {
	return &Worker{
		Pool:         nil,
		ShardIDs:     []int{0},
		PollInterval: 10 * time.Millisecond,
		NotifyCh:     nil,
		InitCursor: func(ctx context.Context, _ *pgxpool.Pool) (int64, error) {
			return 0, nil
		},
		ProcessBatch: process,
		ErrorFn:      func(msg string, err error) {},
	}
}

func TestWorker_ShutdownViaCloseCh(t *testing.T) {
	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		return 0, 0, nil
	})

	closeCh := make(chan struct{})
	done := runWorkerAsync(context.Background(), closeCh, w)

	close(closeCh)
	expectReturn(t, done)
}

func TestWorker_ShutdownViaContext(t *testing.T) {
	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		return 0, 0, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	closeCh := make(chan struct{})
	done := runWorkerAsync(ctx, closeCh, w)

	cancel()
	expectReturn(t, done)
}

func TestWorker_DrainsUntilIdle(t *testing.T) {
	type call struct {
		cursor int64
	}
	var (
		mu    sync.Mutex
		calls []call
	)

	// Return (5, 42, nil), then (3, 50, nil), then (0, 0, nil) forever.
	var callIdx int32
	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, cursor int64, _ []int) (int, int64, error) {
		mu.Lock()
		calls = append(calls, call{cursor: cursor})
		mu.Unlock()
		idx := atomic.AddInt32(&callIdx, 1)
		switch idx {
		case 1:
			return 5, 42, nil
		case 2:
			return 3, 50, nil
		default:
			return 0, 0, nil
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closeCh := make(chan struct{})
	done := runWorkerAsync(ctx, closeCh, w)

	// Wait for at least 3 calls to have happened.
	deadline := time.Now().Add(testRunTimeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(calls)
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(closeCh)
	expectReturn(t, done)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 3 {
		t.Fatalf("expected at least 3 ProcessBatch calls, got %d", len(calls))
	}
	if calls[0].cursor != 0 {
		t.Errorf("first call cursor = %d, want 0", calls[0].cursor)
	}
	if calls[1].cursor != 42 {
		t.Errorf("second call cursor = %d, want 42", calls[1].cursor)
	}
	if calls[2].cursor != 50 {
		t.Errorf("third call cursor = %d, want 50", calls[2].cursor)
	}
}

func TestWorker_IdleWaitWakesFromNotifyCh(t *testing.T) {
	var callCount int32
	var first sync.Once
	firstCallDone := make(chan struct{})

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			first.Do(func() { close(firstCallDone) })
		}
		return 0, 0, nil
	})
	w.PollInterval = 10 * time.Second // long enough that only NotifyCh can wake it
	notifyCh := make(chan struct{}, 1)
	w.NotifyCh = notifyCh

	closeCh := make(chan struct{})
	done := runWorkerAsync(context.Background(), closeCh, w)

	// Wait for the first ProcessBatch call to complete.
	select {
	case <-firstCallDone:
	case <-time.After(testRunTimeout):
		t.Fatal("first ProcessBatch call did not happen")
	}

	// Send a notification — should wake the idle wait and cause another call.
	notifyCh <- struct{}{}

	// Poll for callCount >= 2.
	deadline := time.Now().Add(testRunTimeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&callCount) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&callCount); got < 2 {
		t.Fatalf("expected at least 2 ProcessBatch calls after notify, got %d", got)
	}

	close(closeCh)
	expectReturn(t, done)
}

func TestWorker_IdleWaitWakesFromPollInterval(t *testing.T) {
	var callCount int32

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		atomic.AddInt32(&callCount, 1)
		return 0, 0, nil
	})
	w.PollInterval = 5 * time.Millisecond
	w.NotifyCh = nil // rely entirely on the timer

	closeCh := make(chan struct{})
	done := runWorkerAsync(context.Background(), closeCh, w)

	time.Sleep(100 * time.Millisecond)
	close(closeCh)
	expectReturn(t, done)

	if got := atomic.LoadInt32(&callCount); got < 5 {
		t.Fatalf("expected at least 5 ProcessBatch calls in 100ms with 5ms poll interval, got %d", got)
	}
}

func TestWorker_ProcessBatchErrorLoggedAndRetried(t *testing.T) {
	var callCount int32
	var errorCalls int32
	wantErr := errors.New("boom")

	var (
		errMu      sync.Mutex
		loggedMsgs []string
	)

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			return 0, 0, wantErr
		}
		return 0, 0, nil
	})
	w.ErrorFn = func(msg string, err error) {
		atomic.AddInt32(&errorCalls, 1)
		errMu.Lock()
		loggedMsgs = append(loggedMsgs, msg)
		errMu.Unlock()
	}

	closeCh := make(chan struct{})
	done := runWorkerAsync(context.Background(), closeCh, w)

	// Wait for at least 2 calls (error + retry) and 1 error log.
	deadline := time.Now().Add(testRunTimeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&callCount) >= 2 && atomic.LoadInt32(&errorCalls) >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(closeCh)
	expectReturn(t, done)

	if got := atomic.LoadInt32(&callCount); got < 2 {
		t.Errorf("expected ProcessBatch retried after error, got %d calls", got)
	}
	if got := atomic.LoadInt32(&errorCalls); got < 1 {
		t.Errorf("expected ErrorFn called at least once, got %d", got)
	}
	errMu.Lock()
	defer errMu.Unlock()
	if len(loggedMsgs) == 0 || loggedMsgs[0] != "outbox worker: process batch" {
		t.Errorf("unexpected error log message: %v", loggedMsgs)
	}
}

func TestWorker_ProcessBatchErrorDuringCtxCancel(t *testing.T) {
	var errorCalls int32
	ctx, cancel := context.WithCancel(context.Background())

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
		// Cancel on first call, then return a ctx-derived error.
		cancel()
		return 0, 0, ctx.Err()
	})
	w.ErrorFn = func(msg string, err error) {
		atomic.AddInt32(&errorCalls, 1)
	}

	closeCh := make(chan struct{})
	done := runWorkerAsync(ctx, closeCh, w)
	expectReturn(t, done)

	if got := atomic.LoadInt32(&errorCalls); got != 0 {
		t.Errorf("expected ErrorFn NOT called on ctx-derived error, got %d calls", got)
	}
}

func TestWorker_InitCursorError_RetriesInPlace(t *testing.T) {
	var initCalls int32
	var processCalled int32
	var seenCursor atomic.Int64
	wantErr := errors.New("init boom")
	const successCursor int64 = 12345

	var errorCalls int32

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, cursor int64, _ []int) (int, int64, error) {
		if atomic.CompareAndSwapInt32(&processCalled, 0, 1) {
			seenCursor.Store(cursor)
		}
		return 0, 0, nil
	})
	w.InitCursor = func(ctx context.Context, _ *pgxpool.Pool) (int64, error) {
		n := atomic.AddInt32(&initCalls, 1)
		if n == 1 {
			return 0, wantErr
		}
		return successCursor, nil
	}
	w.ErrorFn = func(msg string, err error) {
		atomic.AddInt32(&errorCalls, 1)
	}

	// Use a very short "retry delay" by cheating: since the constant is 1s
	// internally, we give the test a longer budget to observe retry.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	closeCh := make(chan struct{})
	done := runWorkerAsync(ctx, closeCh, w)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&processCalled) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(closeCh)
	expectReturn(t, done)

	if got := atomic.LoadInt32(&initCalls); got < 2 {
		t.Errorf("expected InitCursor called at least twice (retry), got %d", got)
	}
	if got := atomic.LoadInt32(&errorCalls); got < 1 {
		t.Errorf("expected ErrorFn called at least once for init cursor error, got %d", got)
	}
	if got := atomic.LoadInt32(&processCalled); got != 1 {
		t.Errorf("expected ProcessBatch called after successful init, got processCalled=%d", got)
	}
	if got := seenCursor.Load(); got != successCursor {
		t.Errorf("expected ProcessBatch to see cursor=%d from successful init, got %d", successCursor, got)
	}
}

func TestWorker_InitCursorSetsInitialCursor(t *testing.T) {
	const initialCursor int64 = 9876
	var seen atomic.Int64
	var observed int32

	w := newTestWorker(func(ctx context.Context, _ *pgxpool.Pool, cursor int64, _ []int) (int, int64, error) {
		if atomic.CompareAndSwapInt32(&observed, 0, 1) {
			seen.Store(cursor)
		}
		return 0, 0, nil
	})
	w.InitCursor = func(ctx context.Context, _ *pgxpool.Pool) (int64, error) {
		return initialCursor, nil
	}

	closeCh := make(chan struct{})
	done := runWorkerAsync(context.Background(), closeCh, w)

	deadline := time.Now().Add(testRunTimeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&observed) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(closeCh)
	expectReturn(t, done)

	if got := seen.Load(); got != initialCursor {
		t.Errorf("first ProcessBatch call cursor = %d, want %d", got, initialCursor)
	}
}
