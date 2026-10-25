package workloadmetrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/kofno/bullderdash/internal/store"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"
)

func TestPartialHistoryRecord(t *testing.T) {
	r, err := partialRecord("q", redis.XMessage{ID: "1234-10", Values: map[string]any{
		"jobId": "gone", "failedReason": "expired failure", "returnvalue": `{"ok":true}`,
	}}, "failed")
	if err != nil || r.ID != "gone" || r.FinishedAtMs != 1234 || r.LastError != "expired failure" || r.Data != `{"ok":true}` || r.StreamID != "1234-10" {
		t.Fatalf("%+v %v", r, err)
	}
}

type forbiddenStore struct{}

func (forbiddenStore) Enqueue(store.Record) { panic("metrics reader must not enqueue history") }

// Uses only an explicitly supplied disposable loopback Redis, never production.
func TestHistoryRedisResumeAndTrim(t *testing.T) {
	addr := os.Getenv("HISTORY_TEST_REDIS")
	if addr == "" {
		t.Skip("HISTORY_TEST_REDIS required for real XINFO integration")
	}
	if addr != "127.0.0.1:16389" {
		t.Fatal("only disposable loopback test Redis accepted")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	queue := "history-test-" + time.Now().Format("150405.000000")
	key := "bull:" + queue + ":events"
	metricsOnly := New(client, nil, forbiddenStore{}, Config{})
	metricsOnly.processMessage(ctx, queue, redis.XMessage{ID: "1000-0", Values: map[string]any{"event": "completed", "jobId": "missing"}})
	s, err := store.New(store.Config{Path: filepath.Join(t.TempDir(), "history.db"), BatchSize: 2, FlushInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	writerCtx, stopWriter := context.WithCancel(ctx)
	go s.Run(writerCtx)
	defer func() { stopWriter(); s.Close(); client.Del(ctx, key) }()
	for _, id := range []string{"1000-0", "1000-1", "1000-2"} {
		if err := client.XAdd(ctx, &redis.XAddArgs{Stream: key, ID: id, Values: map[string]any{"event": "completed", "jobId": id}}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	h := New(client, nil, nil, Config{})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- h.historyQueue(runCtx, s, HistoryConfig{StartID: "0-0", ReadBatch: 2, GapCheckInterval: 20 * time.Millisecond}, queue)
	}()
	waitCursor := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			id, e := s.Cursor(ctx, queue)
			if e != nil {
				t.Fatal(e)
			}
			if id == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("cursor never reached %s", want)
	}
	waitCursor("1000-2")
	cancel()
	<-done
	rows, err := s.Search(ctx, store.SearchParams{State: "Completed"})
	if err != nil || len(rows) != 3 {
		t.Fatalf("partial records: %d %v", len(rows), err)
	}
	// Resume a known queue even when its configured new-queue start is '$'.
	client.XAdd(ctx, &redis.XAddArgs{Stream: key, ID: "1000-3", Values: map[string]any{"event": "failed", "jobId": "gone", "failedReason": "boom"}})
	runCtx, cancel = context.WithCancel(ctx)
	go func() {
		done <- h.historyQueue(runCtx, s, HistoryConfig{StartID: "$", ReadBatch: 2, GapCheckInterval: 20 * time.Millisecond}, queue)
	}()
	waitCursor("1000-3")
	cancel()
	<-done
	for _, id := range []string{"1000-4", "1000-5"} {
		client.XAdd(ctx, &redis.XAddArgs{Stream: key, ID: id, Values: map[string]any{"event": "waiting", "jobId": id}})
	}
	if err := client.XTrimMaxLen(ctx, key, 1).Err(); err != nil {
		t.Fatal(err)
	}
	var before, after dto.Metric
	metrics.HistoryGaps.WithLabelValues(queue).Write(&before)
	gap := ""
	if err := h.checkHistoryProgress(ctx, s, queue, key, &gap); err != nil {
		t.Fatal(err)
	}
	if err := h.checkHistoryProgress(ctx, s, queue, key, &gap); err != nil {
		t.Fatal(err)
	}
	metrics.HistoryGaps.WithLabelValues(queue).Write(&after)
	if after.Counter.GetValue()-before.Counter.GetValue() != 1 {
		t.Fatal("trim gap missing or duplicate")
	}
}
