package web

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

const (
	searchResultsPageSize = 50
	searchScanPerState    = 100
	searchMaxScanPages    = 20
)

type searchExplorer interface {
	GetJobsAcrossStatesPage(ctx context.Context, queueName string, offsetPerState, limitPerState int) ([]explorer.JobSummary, error)
}

type searchWindowOption struct {
	Value string
	Label string
}

var searchWindowOptions = []searchWindowOption{
	{Value: "", Label: "Any time"},
	{Value: "15m", Label: "Last 15 minutes"},
	{Value: "1h", Label: "Last hour"},
	{Value: "6h", Label: "Last 6 hours"},
	{Value: "24h", Label: "Last 24 hours"},
	{Value: "7d", Label: "Last 7 days"},
}

type searchWindow struct {
	Value string
	Label string
	Since time.Time
	Set   bool
}

type searchResults struct {
	Jobs         []explorer.JobSummary
	SearchedJobs int
	WindowLabel  string
	HasNextPage  bool
}

func parseSearchWindow(value string, now time.Time) searchWindow {
	switch value {
	case "15m":
		return searchWindow{Value: value, Label: "last 15 minutes", Since: now.Add(-15 * time.Minute), Set: true}
	case "1h":
		return searchWindow{Value: value, Label: "last hour", Since: now.Add(-1 * time.Hour), Set: true}
	case "6h":
		return searchWindow{Value: value, Label: "last 6 hours", Since: now.Add(-6 * time.Hour), Set: true}
	case "24h":
		return searchWindow{Value: value, Label: "last 24 hours", Since: now.Add(-24 * time.Hour), Set: true}
	case "7d":
		return searchWindow{Value: value, Label: "last 7 days", Since: now.Add(-7 * 24 * time.Hour), Set: true}
	default:
		return searchWindow{Value: "", Label: "any time"}
	}
}

func searchJobsAcrossStates(ctx context.Context, exp searchExplorer, queueName, query string, page int, window searchWindow) (searchResults, error) {
	start := (page - 1) * searchResultsPageSize
	endExclusive := start + searchResultsPageSize
	needCount := endExclusive + 1
	queryLower := strings.ToLower(strings.TrimSpace(query))

	filtered := make([]explorer.JobSummary, 0, needCount)
	searchedJobs := 0
	exhausted := false

	for scanPage := 0; scanPage < searchMaxScanPages; scanPage++ {
		offset := scanPage * searchScanPerState
		batch, err := exp.GetJobsAcrossStatesPage(ctx, queueName, offset, searchScanPerState)
		if err != nil {
			return searchResults{}, err
		}

		searchedJobs += len(batch)
		if len(batch) == 0 {
			exhausted = true
			break
		}

		for _, job := range batch {
			if !matchesSearch(job, queryLower, window) {
				continue
			}
			filtered = append(filtered, job)
		}

		if len(batch) < searchScanPerState {
			exhausted = true
		}
		if len(filtered) >= needCount || exhausted {
			break
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	if start >= len(filtered) {
		return searchResults{
			Jobs:         make([]explorer.JobSummary, 0),
			SearchedJobs: searchedJobs,
			WindowLabel:  searchWindowLabel(window),
			HasNextPage:  false,
		}, nil
	}

	end := min(endExclusive, len(filtered))
	hasNext := len(filtered) > end || !exhausted

	return searchResults{
		Jobs:         filtered[start:end],
		SearchedJobs: searchedJobs,
		WindowLabel:  searchWindowLabel(window),
		HasNextPage:  hasNext,
	}, nil
}

func matchesSearch(job explorer.JobSummary, queryLower string, window searchWindow) bool {
	if window.Set && !job.Timestamp.IsZero() && job.Timestamp.Before(window.Since) {
		return false
	}

	return strings.Contains(strings.ToLower(job.ID), queryLower) ||
		strings.Contains(strings.ToLower(job.Name), queryLower) ||
		strings.Contains(strings.ToLower(job.Data), queryLower) ||
		strings.Contains(strings.ToLower(job.Opts), queryLower) ||
		strings.Contains(strings.ToLower(job.FailedReason), queryLower)
}

func searchWindowLabel(window searchWindow) string {
	label := "Searching up to 2000 jobs per state"
	if window.Set {
		return label + " in the " + window.Label
	}
	return label + " across any timestamp"
}
