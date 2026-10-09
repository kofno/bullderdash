package workloadmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/kofno/bullderdash/internal/store"
)

// JobStore is the subset of the SQLite store the collector needs. Kept as an
// interface so the collector can be exercised without a real database.
type JobStore interface {
	Enqueue(store.Record)
}

// maxLineageWalk bounds the parent-chain traversal for flow jobs so a cycle or
// a pathologically deep flow can never block the collector loop.
const maxLineageWalk = 64

// lineage is a resolved (trace root, depth) pair for a single job key.
type lineage struct {
	trace string
	depth int
}

// lineageCache memoizes resolved lineages keyed by the BullMQ job hash key.
// Only flow jobs ever populate it, so growth is slow; a crude size cap keeps
// it bounded without a full LRU.
type lineageCache struct {
	mu  sync.Mutex
	m   map[string]lineage
	cap int
}

func newLineageCache(capacity int) *lineageCache {
	if capacity <= 0 {
		capacity = 10000
	}
	return &lineageCache{m: make(map[string]lineage), cap: capacity}
}

func (c *lineageCache) get(key string) (lineage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *lineageCache) put(key string, v lineage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.cap {
		c.m = make(map[string]lineage, c.cap)
	}
	c.m[key] = v
}

// loadJobFull fetches the whole job hash so a single Redis round-trip can serve
// both the metrics sample and the persisted record.
func (c *Collector) loadJobFull(ctx context.Context, queue, jobID string) (map[string]string, error) {
	key := fmt.Sprintf("%s:%s:%s", c.cfg.QueuePrefix, queue, jobID)

	start := time.Now()
	data, err := c.client.HGetAll(ctx, key).Result()
	metrics.RedisOperationDuration.WithLabelValues("workload_hgetall_job").Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errMissingJob
	}
	return data, nil
}

// sampleFromHash derives the metrics jobSample from a full job hash, matching
// the semantics of parseJobSample (name + completion duration).
func sampleFromHash(data map[string]string) jobSample {
	sample := jobSample{Name: data["name"]}
	processedOn, err1 := strconv.ParseInt(strings.TrimSpace(data["processedOn"]), 10, 64)
	finishedOn, err2 := strconv.ParseInt(strings.TrimSpace(data["finishedOn"]), 10, 64)
	if err1 == nil && err2 == nil && processedOn > 0 && finishedOn > 0 && finishedOn >= processedOn {
		sample.DurationSeconds = float64(finishedOn-processedOn) / 1000
		sample.HasDuration = true
	}
	return sample
}

// buildRecord assembles a store.Record from a job hash, resolving flow lineage
// (trace id + execution depth) with bounded, cached parent-chain traversal.
func (c *Collector) buildRecord(ctx context.Context, queue, jobID, result string, data map[string]string) store.Record {
	rec := store.Record{
		ID:           jobID,
		Queue:        queue,
		Name:         data["name"],
		State:        result,
		Attempts:     int(attemptsMade(data)),
		CreatedAtMs:  parseIntDefault(data["timestamp"], 0),
		FinishedAtMs: parseIntDefault(data["finishedOn"], 0),
		LastError:    data["failedReason"],
		Data:         data["data"],
		Opts:         data["opts"],
	}

	trace, depth := c.resolveLineage(ctx, jobID, data)
	rec.TraceID = trace
	rec.ExecutionDepth = depth
	return rec
}

// resolveLineage returns the trace id and execution depth for a job. An
// explicit trace key in the payload (if configured and present) wins; otherwise
// the trace is the flow root's job id, falling back to the job's own id for
// standalone jobs. Depth is the number of flow ancestors.
func (c *Collector) resolveLineage(ctx context.Context, jobID string, data map[string]string) (string, int) {
	explicit := traceFromData(c.cfg.TraceIDKeys, data["data"])

	parentKey := parentKeyFrom(data)
	if parentKey == "" {
		if explicit != "" {
			return explicit, 0
		}
		return jobID, 0
	}

	rootTrace, parentDepth := c.lineageOf(ctx, parentKey, 0)
	depth := parentDepth + 1
	if explicit != "" {
		return explicit, depth
	}
	return rootTrace, depth
}

// lineageOf resolves the (trace root, depth) for a job identified by its hash
// key, walking up the flow parent chain. Results are cached. The walk is
// bounded by maxLineageWalk to defend against cycles.
func (c *Collector) lineageOf(ctx context.Context, jobKey string, hops int) (string, int) {
	if cached, ok := c.lineage.get(jobKey); ok {
		return cached.trace, cached.depth
	}
	if hops >= maxLineageWalk {
		// Give up walking; treat this node as a root to stay bounded.
		return jobKeyID(jobKey), hops
	}

	start := time.Now()
	fields, err := c.client.HMGet(ctx, jobKey, "parentKey", "parent", "data").Result()
	metrics.RedisOperationDuration.WithLabelValues("workload_hmget_parent").Observe(time.Since(start).Seconds())
	if err != nil || len(fields) != 3 {
		return jobKeyID(jobKey), hops
	}

	hash := map[string]string{
		"parentKey": valueString(fields[0]),
		"parent":    valueString(fields[1]),
		"data":      valueString(fields[2]),
	}
	explicit := traceFromData(c.cfg.TraceIDKeys, hash["data"])

	parentKey := parentKeyFrom(hash)
	var result lineage
	if parentKey == "" {
		if explicit != "" {
			result = lineage{trace: explicit, depth: hops}
		} else {
			result = lineage{trace: jobKeyID(jobKey), depth: hops}
		}
	} else {
		rootTrace, parentDepth := c.lineageOf(ctx, parentKey, hops+1)
		depth := parentDepth + 1
		if explicit != "" {
			result = lineage{trace: explicit, depth: depth}
		} else {
			result = lineage{trace: rootTrace, depth: depth}
		}
	}

	c.lineage.put(jobKey, result)
	return result.trace, result.depth
}

// parentKeyFrom extracts the BullMQ parent job hash key from a job hash. It
// prefers the flat parentKey field and falls back to parsing the parent JSON.
func parentKeyFrom(data map[string]string) string {
	if pk := strings.TrimSpace(data["parentKey"]); pk != "" {
		return pk
	}
	raw := strings.TrimSpace(data["parent"])
	if raw == "" {
		return ""
	}
	var parent struct {
		ID    string `json:"id"`
		Queue string `json:"queue"`
	}
	if err := json.Unmarshal([]byte(raw), &parent); err != nil {
		return ""
	}
	if parent.ID == "" || parent.Queue == "" {
		return ""
	}
	// parent.Queue is already the fully-qualified queue key (e.g. bull:<queue>).
	return fmt.Sprintf("%s:%s", strings.TrimRight(parent.Queue, ":"), parent.ID)
}

// traceFromData returns the first configured trace key found in the job's data
// JSON. Only top-level string/number values are honored.
func traceFromData(keys []string, rawData string) string {
	if len(keys) == 0 || strings.TrimSpace(rawData) == "" {
		return ""
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawData), &payload); err != nil {
		return ""
	}
	for _, k := range keys {
		raw, ok := payload[k]
		if !ok {
			continue
		}
		if v := scalarString(raw); v != "" {
			return v
		}
	}
	return ""
}

// scalarString renders a JSON scalar (string or number) as a plain string,
// ignoring objects, arrays, booleans and nulls.
func scalarString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return strings.TrimSpace(s)
		}
		return ""
	}
	if trimmed[0] == '{' || trimmed[0] == '[' || trimmed == "true" || trimmed == "false" {
		return ""
	}
	return trimmed
}

// jobKeyID returns the trailing job id from a BullMQ hash key (bull:<queue>:<id>).
func jobKeyID(jobKey string) string {
	if i := strings.LastIndex(jobKey, ":"); i >= 0 && i+1 < len(jobKey) {
		return jobKey[i+1:]
	}
	return jobKey
}

func parseIntDefault(s string, def int64) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// attemptsMade reads the BullMQ attempt counter from a job hash. BullMQ v5
// stores it as the abbreviated "atm" field; older releases used the long
// "attemptsMade" name. We honor "atm" first and fall back so the persisted
// attempt count is correct across versions — including jobs that failed one or
// more attempts before finally completing.
func attemptsMade(data map[string]string) int64 {
	if v := strings.TrimSpace(data["atm"]); v != "" {
		return parseIntDefault(v, 0)
	}
	return parseIntDefault(data["attemptsMade"], 0)
}
