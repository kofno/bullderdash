package workloadmetrics

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/kofno/bullderdash/internal/store"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"
)

type fixedHistoryCursor string

func (s fixedHistoryCursor) Cursor(context.Context, string) (string, error) { return string(s), nil }
func (fixedHistoryCursor) EnqueueWait(context.Context, store.Record) error {
	panic("unexpected enqueue")
}

func TestHistoryGapRecreatedAndEmptiedStream(t *testing.T) {
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
	queue := "history-gap-test-" + time.Now().Format("150405.000000")
	key := "bull:" + queue + ":events"
	defer client.Del(ctx, key)
	for _, id := range []string{"2-0", "3-0"} {
		if err := client.XAdd(ctx, &redis.XAddArgs{Stream: key, ID: id, Values: map[string]any{"event": "waiting"}}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	h := New(client, nil, nil, Config{})
	sink := fixedHistoryCursor("1-0")
	gap := ""
	if err := h.checkHistoryProgress(ctx, sink, queue, key, &gap); err != nil {
		t.Fatal(err)
	}
	if gap != "2-0" {
		t.Fatalf("recreated stream with no trim metadata missed: %q", gap)
	}
	if err := client.XTrimMaxLen(ctx, key, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := h.checkHistoryProgress(ctx, sink, queue, key, &gap); err != nil {
		t.Fatal(err)
	}
	if gap != "3-0" {
		t.Fatalf("fully trimmed stream missed: %q", gap)
	}
	var value dto.Metric
	metrics.HistoryGaps.WithLabelValues(queue).Write(&value)
	if value.Counter.GetValue() != 2 {
		t.Fatalf("gap boundaries: %v", value.Counter.GetValue())
	}
}
