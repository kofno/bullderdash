package workloadmetrics

import (
	"context"
	"testing"
)

func TestTraceFromData(t *testing.T) {
	keys := []string{"traceId", "correlationId"}
	cases := []struct {
		name string
		data string
		want string
	}{
		{"string trace", `{"traceId":"abc-123","x":1}`, "abc-123"},
		{"numeric trace", `{"correlationId":42}`, "42"},
		{"first key wins", `{"traceId":"t1","correlationId":"c1"}`, "t1"},
		{"missing", `{"other":"v"}`, ""},
		{"object ignored", `{"traceId":{"nested":1}}`, ""},
		{"array ignored", `{"traceId":[1,2]}`, ""},
		{"bool ignored", `{"traceId":true}`, ""},
		{"empty", ``, ""},
		{"invalid json", `{not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := traceFromData(keys, tc.data); got != tc.want {
				t.Fatalf("traceFromData(%q)=%q want %q", tc.data, got, tc.want)
			}
		})
	}

	if got := traceFromData(nil, `{"traceId":"x"}`); got != "" {
		t.Fatalf("no keys configured should yield empty, got %q", got)
	}
}

func TestParentKeyFrom(t *testing.T) {
	cases := []struct {
		name string
		data map[string]string
		want string
	}{
		{"flat parentKey", map[string]string{"parentKey": "bull:q:7"}, "bull:q:7"},
		{"parent json", map[string]string{"parent": `{"id":"9","queue":"bull:other"}`}, "bull:other:9"},
		{"parent json trailing colon", map[string]string{"parent": `{"id":"9","queue":"bull:other:"}`}, "bull:other:9"},
		{"flat wins", map[string]string{"parentKey": "bull:q:1", "parent": `{"id":"2","queue":"bull:q"}`}, "bull:q:1"},
		{"none", map[string]string{}, ""},
		{"incomplete json", map[string]string{"parent": `{"id":"9"}`}, ""},
		{"bad json", map[string]string{"parent": `nope`}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentKeyFrom(tc.data); got != tc.want {
				t.Fatalf("parentKeyFrom(%v)=%q want %q", tc.data, got, tc.want)
			}
		})
	}
}

func TestJobKeyID(t *testing.T) {
	cases := map[string]string{
		"bull:ProcessDeviceAction:123": "123",
		"123":                          "123",
		"bull:q:":                      "bull:q:",
	}
	for in, want := range cases {
		if got := jobKeyID(in); got != want {
			t.Fatalf("jobKeyID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSampleFromHash(t *testing.T) {
	s := sampleFromHash(map[string]string{
		"name":        "ProcessDeviceAction",
		"processedOn": "1000",
		"finishedOn":  "2500",
	})
	if s.Name != "ProcessDeviceAction" || !s.HasDuration || s.DurationSeconds != 1.5 {
		t.Fatalf("unexpected sample: %+v", s)
	}

	// Missing/invalid timestamps => no duration but name preserved.
	s2 := sampleFromHash(map[string]string{"name": "X"})
	if s2.Name != "X" || s2.HasDuration {
		t.Fatalf("expected no duration, got %+v", s2)
	}

	// finishedOn < processedOn => no duration.
	s3 := sampleFromHash(map[string]string{"processedOn": "5000", "finishedOn": "1000"})
	if s3.HasDuration {
		t.Fatalf("expected no duration for inverted timestamps, got %+v", s3)
	}
}

func TestBuildRecordStandalone(t *testing.T) {
	c := &Collector{cfg: Config{QueuePrefix: "bull", TraceIDKeys: []string{"traceId"}}, lineage: newLineageCache(0)}
	data := map[string]string{
		"name":         "ProcessDeviceAction",
		"data":         `{"deviceId":"dev-1","traceId":"tr-9"}`,
		"opts":         `{"attempts":3}`,
		"timestamp":    "1700000000000",
		"attemptsMade": "2",
		"finishedOn":   "1700000001000",
		"failedReason": "boom",
	}
	rec := c.buildRecord(context.Background(), "ProcessDeviceAction", "job-1", "failed", data)

	if rec.ID != "job-1" || rec.Queue != "ProcessDeviceAction" || rec.Name != "ProcessDeviceAction" {
		t.Fatalf("identity wrong: %+v", rec)
	}
	if rec.State != "failed" || rec.Attempts != 2 || rec.LastError != "boom" {
		t.Fatalf("fields wrong: %+v", rec)
	}
	if rec.CreatedAtMs != 1700000000000 || rec.FinishedAtMs != 1700000001000 {
		t.Fatalf("timestamps wrong: %+v", rec)
	}
	// Explicit trace key present, no parent => depth 0, trace from payload.
	if rec.TraceID != "tr-9" || rec.ExecutionDepth != 0 {
		t.Fatalf("lineage wrong: trace=%q depth=%d", rec.TraceID, rec.ExecutionDepth)
	}
}

func TestBuildRecordStandaloneNoTraceKey(t *testing.T) {
	c := &Collector{cfg: Config{QueuePrefix: "bull"}, lineage: newLineageCache(0)}
	rec := c.buildRecord(context.Background(), "q", "job-7", "completed", map[string]string{"name": "n"})
	// No configured trace key, no parent => trace falls back to own id.
	if rec.TraceID != "job-7" || rec.ExecutionDepth != 0 {
		t.Fatalf("expected self-trace, got trace=%q depth=%d", rec.TraceID, rec.ExecutionDepth)
	}
}
