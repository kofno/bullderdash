package config

import "testing"

func TestStoreSweepPause(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 25},
		{"invalid", 25},
		{"10", 10},
		{"0", 0},
		{"-1", -1},
	} {
		t.Setenv("STORE_SWEEP_PAUSE_MILLIS", tc.value)
		if got := Load().StoreSweepPauseMillis; got != tc.want {
			t.Fatalf("STORE_SWEEP_PAUSE_MILLIS=%q: got %d want %d", tc.value, got, tc.want)
		}
	}
}

func TestHistoryConfiguration(t *testing.T) {
	t.Setenv("HISTORY_START_ID", "")
	t.Setenv("HISTORY_READ_BATCH", "")
	t.Setenv("HISTORY_GAP_CHECK_SECONDS", "")
	cfg := Load()
	if cfg.HistoryStartID != "$" || cfg.HistoryReadBatch != 1000 || cfg.HistoryGapCheckSeconds != 10 {
		t.Fatalf("history defaults: %+v", cfg)
	}
	t.Setenv("HISTORY_START_ID", "0-0")
	t.Setenv("HISTORY_READ_BATCH", "4096")
	t.Setenv("HISTORY_GAP_CHECK_SECONDS", "1")
	cfg = Load()
	if cfg.HistoryStartID != "0-0" || cfg.HistoryReadBatch != 4096 || cfg.HistoryGapCheckSeconds != 1 {
		t.Fatalf("history overrides: %+v", cfg)
	}
}
