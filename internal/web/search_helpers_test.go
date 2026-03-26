package web

import (
	"context"
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

type stubSearchExplorer struct {
	pages map[int][]explorer.JobSummary
}

func (s stubSearchExplorer) GetJobsAcrossStatesPage(ctx context.Context, queueName string, offsetPerState, limitPerState int) ([]explorer.JobSummary, error) {
	return append([]explorer.JobSummary(nil), s.pages[offsetPerState]...), nil
}

func TestParseSearchWindow(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	window := parseSearchWindow("1h", now)
	if !window.Set {
		t.Fatal("expected search window to be set")
	}
	if got, want := window.Since, now.Add(-1*time.Hour); !got.Equal(want) {
		t.Fatalf("since mismatch: got %v want %v", got, want)
	}
}

func TestSearchJobsAcrossStatesFiltersByQueryAndTimeWindow(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	results, err := searchJobsAcrossStates(context.Background(), stubSearchExplorer{
		pages: map[int][]explorer.JobSummary{
			0: {
				{ID: "job-1", Name: "email", Data: `{"account":"abc"}`, Timestamp: now.Add(-10 * time.Minute)},
				{ID: "job-2", Name: "email", Data: `{"account":"old"}`, Timestamp: now.Add(-2 * time.Hour)},
				{ID: "job-3", Name: "billing", Data: `{"account":"xyz"}`, Timestamp: now.Add(-5 * time.Minute)},
			},
		},
	}, "emails", "abc", 1, parseSearchWindow("1h", now))
	if err != nil {
		t.Fatalf("searchJobsAcrossStates returned error: %v", err)
	}

	if got, want := len(results.Jobs), 1; got != want {
		t.Fatalf("job count mismatch: got %d want %d", got, want)
	}
	if got, want := results.Jobs[0].ID, "job-1"; got != want {
		t.Fatalf("job mismatch: got %q want %q", got, want)
	}
}

func TestSearchJobsAcrossStatesSortsNewestFirst(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	results, err := searchJobsAcrossStates(context.Background(), stubSearchExplorer{
		pages: map[int][]explorer.JobSummary{
			0: {
				{ID: "older", Name: "email", Data: "match", Timestamp: now.Add(-30 * time.Minute)},
				{ID: "newer", Name: "email", Data: "match", Timestamp: now.Add(-5 * time.Minute)},
			},
		},
	}, "emails", "match", 1, parseSearchWindow("", now))
	if err != nil {
		t.Fatalf("searchJobsAcrossStates returned error: %v", err)
	}

	if got, want := results.Jobs[0].ID, "newer"; got != want {
		t.Fatalf("expected newest job first, got %q want %q", got, want)
	}
}
