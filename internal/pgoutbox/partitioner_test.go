package pgoutbox

import (
	"context"
	"testing"
	"time"
)

func TestParsePartitionDate_Valid(t *testing.T) {
	cases := []struct {
		name       string
		partName   string
		parent     string
		wantYear   int
		wantMonth  time.Month
		wantDay    int
	}{
		{
			name:      "map stream parent",
			partName:  "cf_map_stream_2025_12_31",
			parent:    "cf_map_stream",
			wantYear:  2025,
			wantMonth: time.December,
			wantDay:   31,
		},
		{
			name:      "binary map stream parent",
			partName:  "cf_binary_map_stream_2026_01_01",
			parent:    "cf_binary_map_stream",
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   1,
		},
		{
			name:      "future stream broker parent",
			partName:  "cf_stream_history_2024_06_15",
			parent:    "cf_stream_history",
			wantYear:  2024,
			wantMonth: time.June,
			wantDay:   15,
		},
		{
			name:      "single-word parent",
			partName:  "events_2025_03_07",
			parent:    "events",
			wantYear:  2025,
			wantMonth: time.March,
			wantDay:   7,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parsePartitionDate(tc.partName, tc.parent)
			if !ok {
				t.Fatalf("parsePartitionDate(%q, %q) = _, false; want ok=true", tc.partName, tc.parent)
			}
			if got.Year() != tc.wantYear || got.Month() != tc.wantMonth || got.Day() != tc.wantDay {
				t.Errorf("parsePartitionDate(%q, %q) = %v, want %d-%02d-%02d",
					tc.partName, tc.parent, got, tc.wantYear, int(tc.wantMonth), tc.wantDay)
			}
		})
	}
}

func TestParsePartitionDate_Invalid(t *testing.T) {
	cases := []struct {
		name     string
		partName string
		parent   string
	}{
		{"empty name", "", "cf_map_stream"},
		{"no date suffix", "cf_map_stream", "cf_map_stream"},
		{"only one component", "stream", "stream"},
		{"short suffix", "cf_map_stream_abc", "cf_map_stream"},
		{"non-date suffix", "cf_map_stream_foo_bar_baz", "cf_map_stream"},
		{"invalid month", "cf_map_stream_2025_13_01", "cf_map_stream"},
		{"invalid day in february", "cf_map_stream_2025_02_30", "cf_map_stream"},
		{"letters in date", "cf_map_stream_abcd_ef_gh", "cf_map_stream"},
		{"too few date components", "cf_map_stream_2025_01", "cf_map_stream"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parsePartitionDate(tc.partName, tc.parent)
			if ok {
				t.Errorf("parsePartitionDate(%q, %q) = %v, true; want ok=false", tc.partName, tc.parent, got)
			}
			if !got.IsZero() {
				t.Errorf("parsePartitionDate(%q, %q) returned non-zero time on failure: %v", tc.partName, tc.parent, got)
			}
		})
	}
}

func TestPartitioner_ShutdownViaCloseCh(t *testing.T) {
	p := &Partitioner{
		Pool:            nil, // never dereferenced because closeCh is already closed when Run starts
		ParentTable:     "test",
		CleanupInterval: time.Hour,
		LookaheadDays:   1,
		RetentionDays:   1,
		ErrorFn:         func(msg string, err error) {},
	}

	closeCh := make(chan struct{})
	close(closeCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(context.Background(), closeCh)
	}()

	select {
	case <-done:
	case <-time.After(testRunTimeout):
		t.Fatal("Partitioner.Run did not return within timeout")
	}
}

func TestPartitioner_ShutdownViaContextCancel(t *testing.T) {
	p := &Partitioner{
		Pool:            nil,
		ParentTable:     "test",
		CleanupInterval: time.Hour,
		LookaheadDays:   1,
		RetentionDays:   1,
		ErrorFn:         func(msg string, err error) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	closeCh := make(chan struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(ctx, closeCh)
	}()

	// Give Run a moment to enter the select, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(testRunTimeout):
		t.Fatal("Partitioner.Run did not return within timeout")
	}
}

// TestPartitioner_DropOldPartitions_RetentionZero_NoOp verifies that when
// RetentionDays is 0, DropOldPartitions returns immediately without touching
// the pool. Without the early-return guard, the method would compute
// cutoff = now and try to query/drop everything before today — equivalent
// to a 1-day retention, not the intended "never drop" semantic.
//
// This test passes a nil Pool: if the guard is removed, the test would
// panic on Pool.Query.
func TestPartitioner_DropOldPartitions_RetentionZero_NoOp(t *testing.T) {
	cases := []struct {
		name          string
		retentionDays int
	}{
		{"zero", 0},
		{"negative", -1},
		{"large negative", -365},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errFnCalled := false
			p := &Partitioner{
				Pool:          nil, // would NPE if guard is missing
				ParentTable:   "test",
				RetentionDays: tc.retentionDays,
				ErrorFn:       func(msg string, err error) { errFnCalled = true },
			}
			// Should be a no-op.
			p.DropOldPartitions(context.Background())
			if errFnCalled {
				t.Errorf("ErrorFn unexpectedly called for RetentionDays=%d", tc.retentionDays)
			}
		})
	}
}

// TestPartitioner_DropOldPartitions_RetentionPositive_AttemptsQuery verifies
// that with a positive RetentionDays, the method does try to query the pool
// (and panics on nil) — i.e. the early-return guard only fires for <= 0.
// This is the negative-case companion to TestPartitioner_DropOldPartitions_RetentionZero_NoOp.
func TestPartitioner_DropOldPartitions_RetentionPositive_AttemptsQuery(t *testing.T) {
	p := &Partitioner{
		Pool:          nil, // intentional — we expect the panic
		ParentTable:   "test",
		RetentionDays: 1,
		ErrorFn:       func(msg string, err error) {},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil Pool with positive RetentionDays, got none — guard may be too aggressive")
		}
	}()
	p.DropOldPartitions(context.Background())
}
