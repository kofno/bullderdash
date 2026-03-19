package web

import (
	"sync"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

type DashboardSnapshot struct {
	Queues    []string
	Stats     []explorer.QueueStats
	UpdatedAt time.Time
}

type DashboardCache struct {
	mu       sync.RWMutex
	snapshot DashboardSnapshot
}

func NewDashboardCache() *DashboardCache {
	return &DashboardCache{
		snapshot: DashboardSnapshot{
			Queues: make([]string, 0),
			Stats:  make([]explorer.QueueStats, 0),
		},
	}
}

func (c *DashboardCache) Get() DashboardSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return DashboardSnapshot{
		Queues:    append([]string(nil), c.snapshot.Queues...),
		Stats:     append([]explorer.QueueStats(nil), c.snapshot.Stats...),
		UpdatedAt: c.snapshot.UpdatedAt,
	}
}

func (c *DashboardCache) Set(queues []string, stats []explorer.QueueStats, updatedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.snapshot = DashboardSnapshot{
		Queues:    append([]string(nil), queues...),
		Stats:     append([]explorer.QueueStats(nil), stats...),
		UpdatedAt: updatedAt,
	}
}
