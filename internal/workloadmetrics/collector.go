package workloadmetrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/redis/go-redis/v9"
)

const (
	unknownJobName = "__unknown__"
	otherJobName   = "__other__"
)

type QueueDiscoverer interface {
	DiscoverQueues(ctx context.Context, prefix string) ([]string, error)
}

type Config struct {
	QueuePrefix         string
	PollInterval        time.Duration
	BlockTimeout        time.Duration
	BatchSize           int64
	MaxJobNamesPerQueue int
	StartID             string
}

type Collector struct {
	client     *redis.Client
	discoverer QueueDiscoverer
	cfg        Config

	mu      sync.Mutex
	queues  []string
	lastIDs map[string]string
	limiter *jobNameLimiter
	now     func() time.Time
}

type jobSample struct {
	Name            string
	DurationSeconds float64
	HasDuration     bool
}

func New(client *redis.Client, discoverer QueueDiscoverer, cfg Config) *Collector {
	cfg = normalizeConfig(cfg)
	return &Collector{
		client:     client,
		discoverer: discoverer,
		cfg:        cfg,
		lastIDs:    make(map[string]string),
		limiter:    newJobNameLimiter(cfg.MaxJobNamesPerQueue),
		now:        time.Now,
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.QueuePrefix == "" {
		cfg.QueuePrefix = "bull"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxJobNamesPerQueue <= 0 {
		cfg.MaxJobNamesPerQueue = 100
	}
	if cfg.StartID == "" {
		cfg.StartID = "$"
	}
	return cfg
}

func (c *Collector) Run(ctx context.Context) {
	nextDiscovery := time.Time{}

	for {
		if ctx.Err() != nil {
			return
		}

		if c.now().After(nextDiscovery) {
			if err := c.refreshQueues(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("workload metrics queue discovery error: %v", err)
			}
			nextDiscovery = c.now().Add(c.cfg.PollInterval)
		}

		if c.queueCount() == 0 {
			if !sleepContext(ctx, c.cfg.PollInterval) {
				return
			}
			continue
		}

		if err := c.readOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, redis.Nil) {
				continue
			}
			metrics.RedisOperationErrors.WithLabelValues("workload_xread").Inc()
			log.Printf("workload metrics event read error: %v", err)
			if !sleepContext(ctx, time.Second) {
				return
			}
		}
	}
}

func (c *Collector) refreshQueues(ctx context.Context) error {
	queues, err := c.discoverer.DiscoverQueues(ctx, c.cfg.QueuePrefix)
	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("workload_discover_queues").Inc()
		return err
	}

	sort.Strings(queues)

	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]struct{}, len(queues))
	for _, queue := range queues {
		seen[queue] = struct{}{}
		if _, ok := c.lastIDs[queue]; !ok {
			c.lastIDs[queue] = c.cfg.StartID
		}
	}
	for queue := range c.lastIDs {
		if _, ok := seen[queue]; !ok {
			delete(c.lastIDs, queue)
		}
	}
	c.queues = append(c.queues[:0], queues...)
	return nil
}

func (c *Collector) queueCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queues)
}

func (c *Collector) readOnce(ctx context.Context) error {
	streams, streamQueues := c.streamArgs()
	if len(streamQueues) == 0 {
		return nil
	}

	start := time.Now()
	results, err := c.client.XRead(ctx, &redis.XReadArgs{
		Streams: streams,
		Count:   c.cfg.BatchSize,
		Block:   c.cfg.BlockTimeout,
	}).Result()
	metrics.RedisOperationDuration.WithLabelValues("workload_xread").Observe(time.Since(start).Seconds())
	if err != nil {
		return err
	}

	for _, stream := range results {
		queue, ok := streamQueues[stream.Stream]
		if !ok {
			continue
		}
		for _, msg := range stream.Messages {
			c.processMessage(ctx, queue, msg)
			c.setLastID(queue, msg.ID)
		}
	}

	return nil
}

func (c *Collector) streamArgs() ([]string, map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	streams := make([]string, 0, len(c.queues)*2)
	ids := make([]string, 0, len(c.queues))
	streamQueues := make(map[string]string, len(c.queues))
	for _, queue := range c.queues {
		stream := fmt.Sprintf("%s:%s:events", c.cfg.QueuePrefix, queue)
		streams = append(streams, stream)
		ids = append(ids, c.lastIDs[queue])
		streamQueues[stream] = queue
	}
	streams = append(streams, ids...)
	return streams, streamQueues
}

func (c *Collector) setLastID(queue, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.lastIDs[queue]; ok {
		c.lastIDs[queue] = id
	}
}

func (c *Collector) processMessage(ctx context.Context, queue string, msg redis.XMessage) {
	event := valueString(msg.Values["event"])
	if event == "" {
		event = "unknown"
	}
	metrics.WorkloadEventsRead.WithLabelValues(queue, event).Inc()
	if lag, ok := eventLagSeconds(msg.ID, c.now()); ok {
		metrics.WorkloadEventLag.WithLabelValues(queue).Set(lag)
	}

	result, ok := terminalResult(event)
	if !ok {
		return
	}

	jobID := valueString(msg.Values["jobId"])
	if jobID == "" {
		metrics.WorkloadEventsDropped.WithLabelValues(queue, "missing_job_id").Inc()
		return
	}

	sample, err := c.loadJobSample(ctx, queue, jobID)
	if err != nil {
		metrics.WorkloadJobLookupErrors.WithLabelValues(queue, lookupErrorReason(err)).Inc()
	}

	name := c.limiter.label(queue, sample.Name)
	metrics.WorkloadJobsFinished.WithLabelValues(queue, name, result).Inc()
	if sample.HasDuration {
		metrics.WorkloadJobCompletionDuration.WithLabelValues(queue, name, result).Observe(sample.DurationSeconds)
	}
}

func (c *Collector) loadJobSample(ctx context.Context, queue, jobID string) (jobSample, error) {
	key := fmt.Sprintf("%s:%s:%s", c.cfg.QueuePrefix, queue, jobID)

	start := time.Now()
	values, err := c.client.HMGet(ctx, key, "name", "processedOn", "finishedOn").Result()
	metrics.RedisOperationDuration.WithLabelValues("workload_hmget_job").Observe(time.Since(start).Seconds())
	if err != nil {
		return jobSample{}, err
	}

	sample, err := parseJobSample(values)
	if err != nil {
		return jobSample{}, err
	}
	return sample, nil
}

func terminalResult(event string) (string, bool) {
	switch event {
	case "completed":
		return "completed", true
	case "failed":
		return "failed", true
	default:
		return "", false
	}
}

func lookupErrorReason(err error) string {
	switch {
	case errors.Is(err, errMissingJob):
		return "missing_job"
	case errors.Is(err, errInvalidTimestamp):
		return "invalid_timestamp"
	default:
		return "redis_error"
	}
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type jobNameLimiter struct {
	mu       sync.Mutex
	maxNames int
	seen     map[string]map[string]struct{}
}

func newJobNameLimiter(maxNames int) *jobNameLimiter {
	return &jobNameLimiter{
		maxNames: maxNames,
		seen:     make(map[string]map[string]struct{}),
	}
}

func (l *jobNameLimiter) label(queue, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = unknownJobName
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	names, ok := l.seen[queue]
	if !ok {
		names = make(map[string]struct{})
		l.seen[queue] = names
	}
	if _, ok := names[name]; ok {
		return name
	}
	if len(names) >= l.maxNames {
		return otherJobName
	}
	names[name] = struct{}{}
	return name
}
