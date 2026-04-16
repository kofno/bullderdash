package workloadmetrics

import (
	"errors"
	"testing"
	"time"
)

func TestParseJobSample(t *testing.T) {
	sample, err := parseJobSample([]interface{}{"send-email", "1000", "2500"})
	if err != nil {
		t.Fatalf("parseJobSample returned error: %v", err)
	}
	if sample.Name != "send-email" {
		t.Fatalf("expected name send-email, got %q", sample.Name)
	}
	if sample.DurationSeconds != 1.5 {
		t.Fatalf("expected duration 1.5, got %v", sample.DurationSeconds)
	}
	if !sample.HasDuration {
		t.Fatal("expected sample to have duration")
	}
}

func TestParseJobSampleMissingJob(t *testing.T) {
	_, err := parseJobSample([]interface{}{nil, nil, nil})
	if !errors.Is(err, errMissingJob) {
		t.Fatalf("expected errMissingJob, got %v", err)
	}
}

func TestParseJobSampleInvalidTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		values []interface{}
	}{
		{name: "missing processedOn", values: []interface{}{"job", nil, "2500"}},
		{name: "missing finishedOn", values: []interface{}{"job", "1000", nil}},
		{name: "non-numeric processedOn", values: []interface{}{"job", "nope", "2500"}},
		{name: "finished before processed", values: []interface{}{"job", "2500", "1000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseJobSample(tt.values)
			if !errors.Is(err, errInvalidTimestamp) {
				t.Fatalf("expected errInvalidTimestamp, got %v", err)
			}
		})
	}
}

func TestParseJobSampleReturnsNameWithInvalidTimestamp(t *testing.T) {
	sample, err := parseJobSample([]interface{}{"send-email", "nope", "2500"})
	if !errors.Is(err, errInvalidTimestamp) {
		t.Fatalf("expected errInvalidTimestamp, got %v", err)
	}
	if sample.Name != "send-email" {
		t.Fatalf("expected name send-email, got %q", sample.Name)
	}
	if sample.HasDuration {
		t.Fatal("expected invalid timestamp sample to skip duration")
	}
}

func TestTerminalResult(t *testing.T) {
	tests := []struct {
		event      string
		wantResult string
		wantOK     bool
	}{
		{event: "completed", wantResult: "completed", wantOK: true},
		{event: "failed", wantResult: "failed", wantOK: true},
		{event: "progress", wantOK: false},
	}

	for _, tt := range tests {
		gotResult, gotOK := terminalResult(tt.event)
		if gotResult != tt.wantResult || gotOK != tt.wantOK {
			t.Fatalf("terminalResult(%q) = %q, %t; want %q, %t", tt.event, gotResult, gotOK, tt.wantResult, tt.wantOK)
		}
	}
}

func TestEventLagSeconds(t *testing.T) {
	now := time.UnixMilli(2500)
	lag, ok := eventLagSeconds("1000-0", now)
	if !ok {
		t.Fatal("expected lag to be parsed")
	}
	if lag != 1.5 {
		t.Fatalf("expected lag 1.5, got %v", lag)
	}
}

func TestJobNameLimiter(t *testing.T) {
	limiter := newJobNameLimiter(2)

	if got := limiter.label("emails", "send"); got != "send" {
		t.Fatalf("expected send, got %q", got)
	}
	if got := limiter.label("emails", "digest"); got != "digest" {
		t.Fatalf("expected digest, got %q", got)
	}
	if got := limiter.label("emails", "send"); got != "send" {
		t.Fatalf("expected existing name send to remain allowed, got %q", got)
	}
	if got := limiter.label("emails", "receipt"); got != otherJobName {
		t.Fatalf("expected overflow name %q, got %q", otherJobName, got)
	}
	if got := limiter.label("billing", "receipt"); got != "receipt" {
		t.Fatalf("expected separate queue budget, got %q", got)
	}
	if got := limiter.label("billing", " "); got != unknownJobName {
		t.Fatalf("expected unknown name label, got %q", got)
	}
}
