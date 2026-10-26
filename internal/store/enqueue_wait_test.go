package store

import (
	"context"
	"testing"
	"time"
)

func TestEnqueueWaitWakesAllAvailableSlots(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 3})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range 3 {
		if err := s.EnqueueWait(ctx, Record{}); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 3)
	for range 3 {
		go func() { results <- s.EnqueueWait(ctx, Record{}) }()
	}
	time.Sleep(20 * time.Millisecond)
	// Coalesce several receives into one notification before any waiter runs.
	s.queueMu.Lock()
	for range 3 {
		<-s.recs
	}
	select {
	case s.space <- struct{}{}:
	default:
	}
	s.queueMu.Unlock()
	for range 3 {
		if err := <-results; err != nil {
			t.Fatalf("available slots left idle: %v", err)
		}
	}
	if len(s.recs) != 3 {
		t.Fatalf("buffer depth %d", len(s.recs))
	}
}
