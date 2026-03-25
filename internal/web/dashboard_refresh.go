package web

import (
	"context"
	"errors"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

type dashboardRefresher interface {
	DiscoverQueues(ctx context.Context, prefix string) ([]string, error)
	GetQueueStatsFast(ctx context.Context, queuePrefix string, queues []string) ([]explorer.QueueStats, error)
}

// RefreshDashboardCache updates the dashboard snapshot. If queue discovery times
// out but we already have a cached queue list, reuse that list so counts can
// continue refreshing instead of leaving the dashboard stale indefinitely.
func RefreshDashboardCache(ctx context.Context, exp dashboardRefresher, prefix string, cache *DashboardCache) error {
	queues, err := exp.DiscoverQueues(ctx, prefix)
	if err != nil {
		snapshot := cache.Get()
		if len(snapshot.Queues) == 0 || !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		queues = snapshot.Queues
	}

	stats, err := exp.GetQueueStatsFast(ctx, prefix, queues)
	if err != nil {
		return err
	}

	cache.Set(queues, stats, time.Now())
	return nil
}
