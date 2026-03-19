package web

import (
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

func TestDashboardCacheCopiesStoredSlices(t *testing.T) {
	cache := NewDashboardCache()
	updatedAt := time.Unix(1700000000, 0)

	queues := []string{"emails", "orders"}
	stats := []explorer.QueueStats{
		{Name: "emails", Wait: 4, Total: 4},
	}

	cache.Set(queues, stats, updatedAt)

	queues[0] = "mutated"
	stats[0].Name = "mutated"

	snapshot := cache.Get()
	if got, want := snapshot.Queues[0], "emails"; got != want {
		t.Fatalf("queue copy mismatch: got %q want %q", got, want)
	}
	if got, want := snapshot.Stats[0].Name, "emails"; got != want {
		t.Fatalf("stats copy mismatch: got %q want %q", got, want)
	}
	if !snapshot.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updatedAt mismatch: got %v want %v", snapshot.UpdatedAt, updatedAt)
	}
}

func TestDashboardCacheGetReturnsCopies(t *testing.T) {
	cache := NewDashboardCache()
	cache.Set([]string{"billing"}, []explorer.QueueStats{{Name: "billing", Total: 9}}, time.Unix(1700000500, 0))

	snapshot := cache.Get()
	snapshot.Queues[0] = "mutated"
	snapshot.Stats[0].Name = "mutated"

	again := cache.Get()
	if got, want := again.Queues[0], "billing"; got != want {
		t.Fatalf("queue mutation leaked: got %q want %q", got, want)
	}
	if got, want := again.Stats[0].Name, "billing"; got != want {
		t.Fatalf("stats mutation leaked: got %q want %q", got, want)
	}
}
