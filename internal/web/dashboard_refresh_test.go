package web

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

type stubDashboardRefresher struct {
	discoverQueues func(ctx context.Context, prefix string) ([]string, error)
	getQueueStats  func(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error)
}

func (s stubDashboardRefresher) DiscoverQueues(ctx context.Context, prefix string) ([]string, error) {
	return s.discoverQueues(ctx, prefix)
}

func (s stubDashboardRefresher) GetQueueStatsFast(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error) {
	return s.getQueueStats(ctx, queuePrefix, queues)
}

func TestRefreshDashboardCacheFallsBackToCachedQueuesOnDeadlineExceeded(t *testing.T) {
	cache := NewDashboardCache()
	cache.Set(
		[]string{"emails"},
		[]explorer.QueueStats{{Name: "emails", Total: 1}},
		time.Unix(1700000000, 0),
	)

	var gotQueues []string
	err := RefreshDashboardCache(context.Background(), stubDashboardRefresher{
		discoverQueues: func(ctx context.Context, prefix string) ([]string, error) {
			return nil, context.DeadlineExceeded
		},
		getQueueStats: func(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error) {
			gotQueues = append([]string(nil), queues...)
			return []explorer.QueueStats{{Name: "emails", Total: 9}}, nil
		},
	}, "bull", cache)
	if err != nil {
		t.Fatalf("RefreshDashboardCache returned error: %v", err)
	}

	if len(gotQueues) != 1 || gotQueues[0] != "emails" {
		t.Fatalf("expected cached queues to be reused, got %v", gotQueues)
	}

	snapshot := cache.Get()
	if got, want := snapshot.Stats[0].Total, int64(9); got != want {
		t.Fatalf("stats total mismatch: got %d want %d", got, want)
	}
}

func TestRefreshDashboardCacheReturnsDiscoveryErrorWithoutCachedQueues(t *testing.T) {
	cache := NewDashboardCache()
	wantErr := context.DeadlineExceeded

	err := RefreshDashboardCache(context.Background(), stubDashboardRefresher{
		discoverQueues: func(ctx context.Context, prefix string) ([]string, error) {
			return nil, wantErr
		},
		getQueueStats: func(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error) {
			t.Fatal("GetQueueStatsFast should not be called when there is no cached queue list")
			return nil, nil
		},
	}, "bull", cache)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestRefreshDashboardCacheReturnsNonDeadlineDiscoveryError(t *testing.T) {
	cache := NewDashboardCache()
	cache.Set(
		[]string{"emails"},
		[]explorer.QueueStats{{Name: "emails", Total: 1}},
		time.Unix(1700000000, 0),
	)
	wantErr := errors.New("redis unavailable")

	err := RefreshDashboardCache(context.Background(), stubDashboardRefresher{
		discoverQueues: func(ctx context.Context, prefix string) ([]string, error) {
			return nil, wantErr
		},
		getQueueStats: func(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error) {
			t.Fatal("GetQueueStatsFast should not be called for non-timeout discovery failures")
			return nil, nil
		},
	}, "bull", cache)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
