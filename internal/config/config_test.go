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
