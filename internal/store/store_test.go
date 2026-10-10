package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "history.db")
	s, err := New(Config{Path: dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// waitForID polls Search until a record with id appears or the deadline passes.
func waitForID(t *testing.T, s *Store, p SearchParams, wantID string) []SearchRow {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := s.Search(ctx, p)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		for _, r := range rows {
			if r.ID == wantID {
				return rows
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("record %q never appeared", wantID)
	return nil
}

func TestStoreLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	now := time.Now().UnixMilli()
	s.Enqueue(Record{
		ID:             "job-1",
		Queue:          "ProcessDeviceAction",
		Name:           "ProcessDeviceAction",
		State:          "completed",
		Attempts:       1,
		ExecutionDepth: 0,
		TraceID:        "trace-abc",
		CreatedAtMs:    now - 1000,
		FinishedAtMs:   now,
		Data:           `{"deviceId":"dev-42","action":"reboot"}`,
		Opts:           `{"attempts":3}`,
	})
	s.Enqueue(Record{
		ID:             "job-2",
		Queue:          "ProcessDeviceAction",
		Name:           "ProcessDeviceAction",
		State:          "failed",
		Attempts:       3,
		ExecutionDepth: 1,
		TraceID:        "trace-abc",
		CreatedAtMs:    now - 500,
		FinishedAtMs:   now + 10,
		LastError:      "device offline",
		Data:           `{"deviceId":"dev-42","action":"status"}`,
	})

	// FTS over payload
	rows := waitForID(t, s, SearchParams{Query: "dev-42"}, "job-2")
	if len(rows) < 2 {
		t.Fatalf("expected both jobs for payload query, got %d", len(rows))
	}

	// State predicate
	failed, err := s.Search(ctx, SearchParams{State: "Failed"})
	if err != nil {
		t.Fatalf("Search state: %v", err)
	}
	if len(failed) != 1 || failed[0].ID != "job-2" {
		t.Fatalf("state filter wrong: %+v", failed)
	}

	// Trace lineage ordering by depth
	lineage, err := s.Search(ctx, SearchParams{TraceID: "trace-abc"})
	if err != nil {
		t.Fatalf("Search trace: %v", err)
	}
	if len(lineage) != 2 {
		t.Fatalf("expected 2 in lineage, got %d", len(lineage))
	}

	// Detail with payload
	detail, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail == nil || detail.Data == "" {
		t.Fatalf("expected job-1 detail with data, got %+v", detail)
	}

	// Sweep: completed TTL 0 keeps nothing old enough; use tiny TTLs to prove deletion.
	deleted, err := s.Sweep(ctx, Retention{CompletedTTL: time.Nanosecond})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected completed job swept, deleted=%d", deleted)
	}
	// Failed job must survive a completed-only sweep.
	if d, _ := s.Get(ctx, "job-2"); d == nil {
		t.Fatalf("failed job-2 should survive completed sweep")
	}
}

func TestTopJobNames(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	now := time.Now().UnixMilli()
	// alpha: 3 completed on Legacy. beta: 2 completed on Legacy. gamma: 1 failed on Workflow.
	for i := 0; i < 3; i++ {
		s.Enqueue(Record{ID: "a" + string(rune('0'+i)), Queue: "Legacy", Name: "alpha", State: "completed", Attempts: 1, FinishedAtMs: now, Data: `{"k":1}`})
	}
	for i := 0; i < 2; i++ {
		s.Enqueue(Record{ID: "b" + string(rune('0'+i)), Queue: "Legacy", Name: "beta", State: "completed", Attempts: 1, FinishedAtMs: now, Data: `{"k":2}`})
	}
	s.Enqueue(Record{ID: "g0", Queue: "Workflow", Name: "gamma", State: "failed", Attempts: 1, FinishedAtMs: now, Data: `{"k":3}`})

	waitForID(t, s, SearchParams{Query: "gamma"}, "g0")

	// Global ordering by count desc.
	all, err := s.TopJobNames(ctx, JobNameStatsParams{})
	if err != nil {
		t.Fatalf("TopJobNames: %v", err)
	}
	if len(all) != 3 || all[0].Name != "alpha" || all[0].Count != 3 {
		t.Fatalf("expected alpha(3) first, got %+v", all)
	}
	if all[1].Name != "beta" || all[1].Count != 2 {
		t.Fatalf("expected beta(2) second, got %+v", all)
	}

	// Queue filter.
	wf, err := s.TopJobNames(ctx, JobNameStatsParams{Queue: "Workflow"})
	if err != nil {
		t.Fatalf("TopJobNames queue: %v", err)
	}
	if len(wf) != 1 || wf[0].Name != "gamma" {
		t.Fatalf("queue filter wrong: %+v", wf)
	}

	// State filter (case-insensitive via titleState).
	failed, err := s.TopJobNames(ctx, JobNameStatsParams{State: "failed"})
	if err != nil {
		t.Fatalf("TopJobNames state: %v", err)
	}
	if len(failed) != 1 || failed[0].Name != "gamma" {
		t.Fatalf("state filter wrong: %+v", failed)
	}

	// Limit clamp.
	one, err := s.TopJobNames(ctx, JobNameStatsParams{Limit: 1})
	if err != nil {
		t.Fatalf("TopJobNames limit: %v", err)
	}
	if len(one) != 1 || one[0].Name != "alpha" {
		t.Fatalf("limit wrong: %+v", one)
	}

	// since_ms excludes older rows.
	none, err := s.TopJobNames(ctx, JobNameStatsParams{SinceMs: now + 1_000_000})
	if err != nil {
		t.Fatalf("TopJobNames since_ms: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no rows past future since_ms, got %+v", none)
	}
}

func TestSearchErroredFilter(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	now := time.Now().UnixMilli()
	// Clean completion: single attempt, no error -> excluded by errored filter.
	s.Enqueue(Record{ID: "clean", Queue: "q", Name: "n", State: "completed", Attempts: 1, FinishedAtMs: now, Data: `{"k":1}`})
	// Retried then completed: attempts > 1, no error -> included.
	s.Enqueue(Record{ID: "retried", Queue: "q", Name: "n", State: "completed", Attempts: 2, FinishedAtMs: now, Data: `{"k":2}`})
	// Errored then completed: attempts 1 but carries last_error -> included.
	s.Enqueue(Record{ID: "errored", Queue: "q", Name: "n", State: "completed", Attempts: 1, LastError: "boom", FinishedAtMs: now, Data: `{"k":3}`})

	waitForID(t, s, SearchParams{Errored: true}, "retried")
	rows, err := s.Search(ctx, SearchParams{Errored: true})
	if err != nil {
		t.Fatalf("Search errored: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if got["clean"] {
		t.Fatalf("clean completion must be excluded by errored filter: %+v", rows)
	}
	if !got["retried"] || !got["errored"] {
		t.Fatalf("errored filter must include retried and errored jobs: %+v", rows)
	}
}

func TestSweepFailedTTLAndMaxRows(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	old := time.Now().Add(-48 * time.Hour).UnixMilli()
	s.Enqueue(Record{ID: "f-old", Queue: "q", Name: "n", State: "failed", FinishedAtMs: old, Data: `{"k":1}`})
	s.Enqueue(Record{ID: "f-new", Queue: "q", Name: "n", State: "failed", FinishedAtMs: time.Now().UnixMilli(), Data: `{"k":2}`})
	waitForID(t, s, SearchParams{State: "Failed"}, "f-new")
	waitForID(t, s, SearchParams{State: "Failed"}, "f-old")

	// FailedTTL of 24h removes the 48h-old failure but keeps the fresh one.
	deleted, err := s.Sweep(ctx, Retention{FailedTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("Sweep failed TTL: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 old failure swept, got %d", deleted)
	}
	if d, _ := s.Get(ctx, "f-old"); d != nil {
		t.Fatalf("f-old should have been swept")
	}
	if d, _ := s.Get(ctx, "f-new"); d == nil {
		t.Fatalf("f-new should survive")
	}

	// MaxRows hard cap keeps only the newest row.
	base := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		s.Enqueue(Record{
			ID:           "cap-" + string(rune('a'+i)),
			Queue:        "q",
			Name:         "n",
			State:        "completed",
			FinishedAtMs: base + int64(i),
		})
	}
	waitForID(t, s, SearchParams{State: "Completed"}, "cap-e")

	if _, err := s.Sweep(ctx, Retention{MaxRows: 1}); err != nil {
		t.Fatalf("Sweep maxrows: %v", err)
	}
	rows, err := s.Search(ctx, SearchParams{State: "Completed", Limit: 100})
	if err != nil {
		t.Fatalf("Search after cap: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "cap-e" {
		t.Fatalf("MaxRows cap wrong, got %+v", rows)
	}
}
