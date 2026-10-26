package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNumericStreamIDs(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"9-10", "10-0", -1}, {"10-9", "10-10", -1}, {"10-10", "10-2", 1}, {"10-0", "10-0", 0},
	} {
		got, err := CompareStreamIDs(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %d %v", tc, got, err)
		}
	}
	if _, err := CompareStreamIDs("invalid", "1-0"); err == nil {
		t.Fatal("invalid ID accepted")
	}
}

func TestCursorAtomicityAndReplay(t *testing.T) {
	s := quietStore(t, Config{})
	ctx := context.Background()
	for _, batch := range [][]Record{
		{{ID: "job", Queue: "q", State: "Completed", StreamID: "9-10"}},
		{{Queue: "q", StreamID: "10-2", CheckpointOnly: true}, {Queue: "q", StreamID: "10-10", CheckpointOnly: true}},
		{{ID: "job", Queue: "q", State: "Completed", StreamID: "9-10"}},
	} {
		if err := s.writeBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	id, err := s.Cursor(ctx, "q")
	if err != nil || id != "10-10" {
		t.Fatalf("cursor %q %v", id, err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON job_history BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	err = s.writeBatch(ctx, []Record{{Queue: "q", StreamID: "11-0", CheckpointOnly: true}, {ID: "bad", Queue: "q", StreamID: "11-1"}})
	if err == nil {
		t.Fatal("expected rollback")
	}
	id, err = s.Cursor(ctx, "q")
	if err != nil || id != "10-10" {
		t.Fatalf("failed batch advanced cursor %q %v", id, err)
	}
}

func TestEnqueueWaitBackpressureAndCancellation(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 1})
	if err := s.EnqueueWait(context.Background(), Record{ID: "first"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.EnqueueWait(ctx, Record{ID: "cancelled"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- s.EnqueueWait(context.Background(), Record{ID: "second"}) }()
	select {
	case <-result:
		t.Fatal("did not block")
	case <-time.After(20 * time.Millisecond):
	}
	<-s.recs
	s.received()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestFailedCheckpointBlocksLaterCursorOnShutdown(t *testing.T) {
	s := quietStore(t, Config{BatchSize: 1, WriteBuffer: 4})
	if _, err := s.db.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON job_history BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}

	s.EnqueueWait(context.Background(), Record{ID: "bad", Queue: "q", StreamID: "1-0"})
	s.EnqueueWait(context.Background(), Record{Queue: "q", StreamID: "2-0", CheckpointOnly: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go s.Run(ctx)
	<-s.done
	id, err := s.Cursor(context.Background(), "q")
	if err != nil || id != "" {
		t.Fatalf("later checkpoint skipped failed record: %q %v", id, err)
	}
}

func TestPermanentHistoryErrorRetainsBatchUntilRecovery(t *testing.T) {
	s := quietStore(t, Config{BatchSize: 1, FlushInterval: time.Millisecond})
	_, err := s.db.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON job_history BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	defer func() { cancel(); <-s.done }()
	if err := s.EnqueueWait(ctx, Record{ID: "retry", Queue: "q", StreamID: "1-0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueWait(ctx, Record{Queue: "q", StreamID: "2-0", CheckpointOnly: true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	id, err := s.Cursor(ctx, "q")
	if err != nil || id != "" {
		t.Fatalf("failed batch advanced %q %v", id, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_history`); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		id, err = s.Cursor(ctx, "q")
		if err != nil {
			t.Fatal(err)
		}
		if id == "2-0" {
			if _, err := s.Get(ctx, "retry"); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("history never recovered from permanent write error")
}
