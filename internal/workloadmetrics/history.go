package workloadmetrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/kofno/bullderdash/internal/store"
	"github.com/redis/go-redis/v9"
)

type HistoryStore interface {
	EnqueueWait(context.Context, store.Record) error
	Cursor(context.Context, string) (string, error)
}

type HistoryConfig struct {
	StartID          string
	ReadBatch        int64
	GapCheckInterval time.Duration
}

// RunHistory owns a separate stream reader. Its cursors are never used for
// live metrics. Discovery is shared with the metrics collector.
func (c *Collector) RunHistory(ctx context.Context, sink HistoryStore, cfg HistoryConfig) {
	if cfg.StartID == "" {
		cfg.StartID = "$"
	}
	if cfg.ReadBatch <= 0 {
		cfg.ReadBatch = 1000
	}
	if cfg.GapCheckInterval <= 0 {
		cfg.GapCheckInterval = 10 * time.Second
	}
	seen := make(map[string]struct{})
	var workers sync.WaitGroup
	defer workers.Wait()
	for ctx.Err() == nil {
		c.mu.Lock()
		queues := append([]string(nil), c.queues...)
		c.mu.Unlock()
		for _, queue := range queues {
			if _, ok := seen[queue]; ok {
				continue
			}
			seen[queue] = struct{}{}
			workers.Add(1)
			go func() {
				defer workers.Done()
				// Give history its own lineage cache; metrics does not resolve it.
				h := &Collector{client: c.client, cfg: c.cfg, lineage: newLineageCache(0)}
				for ctx.Err() == nil {
					if err := h.historyQueue(ctx, sink, cfg, queue); err != nil && ctx.Err() == nil {
						log.Printf("history queue=%q paused; will resume committed cursor: %v", queue, err)
						metrics.RedisOperationErrors.WithLabelValues("history_ingest").Inc()
						if !sleepContext(ctx, time.Second) {
							return
						}
					}
				}
			}()
		}
		if !sleepContext(ctx, time.Second) {
			return
		}
	}
}

func streamMillis(id string) (int64, error) {
	if _, err := store.CompareStreamIDs(id, id); err != nil {
		return 0, err
	}
	ms, err := strconv.ParseInt(strings.Split(id, "-")[0], 10, 64)
	return ms, err
}

func (c *Collector) streamInfo(ctx context.Context, key string) (*redis.XInfoStream, error) {
	start := time.Now()
	info, err := c.client.XInfoStream(ctx, key).Result()
	metrics.RedisOperationDuration.WithLabelValues("history_xinfo").Observe(time.Since(start).Seconds())
	return info, err
}

// A new '$' cursor is resolved once to a concrete tail and persisted BEFORE
// reading. Repeated XREAD '$' would otherwise skip events between calls.
func (c *Collector) historyQueue(ctx context.Context, sink HistoryStore, cfg HistoryConfig, queue string) error {
	key := fmt.Sprintf("%s:%s:events", c.cfg.QueuePrefix, queue)
	cursor, err := sink.Cursor(ctx, queue)
	if err != nil {
		return err
	}
	if cursor == "" {
		cursor = cfg.StartID
		if cursor == "$" {
			info, err := c.streamInfo(ctx, key)
			if err != nil {
				if !strings.Contains(err.Error(), "no such key") {
					return err
				}
				cursor = "0-0"
			} else {
				cursor = info.LastGeneratedID
			}
		}
		if cursor == "0" {
			cursor = "0-0"
		}
		if _, err := streamMillis(cursor); err != nil {
			return err
		}
		if err := sink.EnqueueWait(ctx, store.Record{Queue: queue, StreamID: cursor, CheckpointOnly: true}); err != nil {
			return err
		}
		// Do not read beyond a '$' baseline until it is durable. Otherwise a
		// failure before its commit could resolve '$' again and skip history.
		for {
			committed, err := sink.Cursor(ctx, queue)
			if err != nil {
				return err
			}
			if committed != "" {
				cmp, err := store.CompareStreamIDs(committed, cursor)
				if err != nil {
					return err
				}
				if cmp >= 0 {
					break
				}
			}
			if !sleepContext(ctx, 10*time.Millisecond) {
				return ctx.Err()
			}
		}
	}
	// Monitoring runs independently so lag continues updating while admission
	// is blocked. It uses the durable cursor, not the read-ahead position.
	monitorCtx, stop := context.WithCancel(ctx)
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		lastGap := ""
		for monitorCtx.Err() == nil {
			if err := c.checkHistoryProgress(monitorCtx, sink, queue, key, &lastGap); err != nil && monitorCtx.Err() == nil {
				metrics.RedisOperationErrors.WithLabelValues("history_gap_check").Inc()
				log.Printf("history progress queue=%q: %v", queue, err)
			}
			if !sleepContext(monitorCtx, cfg.GapCheckInterval) {
				return
			}
		}
	}()
	defer func() { stop(); <-monitorDone }()
	for ctx.Err() == nil {
		start := time.Now()
		streams, err := c.client.XRead(ctx, &redis.XReadArgs{Streams: []string{key, cursor}, Count: cfg.ReadBatch, Block: time.Second}).Result()
		metrics.RedisOperationDuration.WithLabelValues("history_xread").Observe(time.Since(start).Seconds())
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return err
		}
		for _, stream := range streams {
			for i, msg := range stream.Messages {
				rec := store.Record{Queue: queue, StreamID: msg.ID, CheckpointOnly: true}
				if result, ok := terminalResult(valueString(msg.Values["event"])); ok {
					id := valueString(msg.Values["jobId"])
					if id == "" {
						return fmt.Errorf("terminal event %s has no jobId", msg.ID)
					}
					start := time.Now()
					data, err := c.client.HGetAll(ctx, fmt.Sprintf("%s:%s:%s", c.cfg.QueuePrefix, queue, id)).Result()
					metrics.RedisOperationDuration.WithLabelValues("history_hgetall").Observe(time.Since(start).Seconds())
					if err != nil {
						return err
					}
					if len(data) == 0 {
						rec, err = partialRecord(queue, msg, result)
						if err != nil {
							return err
						}
						metrics.HistoryPartial.WithLabelValues(queue).Inc()
					} else {
						rec = c.buildRecord(ctx, queue, id, result, data)
						rec.StreamID = msg.ID
					}
				}
				if rec.CheckpointOnly && i != len(stream.Messages)-1 {
					cursor = msg.ID
					continue
				}
				// Terminal observations and ordered cursor-only events share the
				// same writer FIFO, so no checkpoint can pass an unwritten job.
				if err := sink.EnqueueWait(ctx, rec); err != nil {
					return err
				}
				cursor = msg.ID
			}
		}
	}
	return ctx.Err()
}

func partialRecord(queue string, msg redis.XMessage, result string) (store.Record, error) {
	ms, err := streamMillis(msg.ID)
	if err != nil {
		return store.Record{}, err
	}
	return store.Record{Queue: queue, ID: valueString(msg.Values["jobId"]), State: result,
		StreamID: msg.ID, FinishedAtMs: ms, LastError: valueString(msg.Values["failedReason"]),
		Data: valueString(msg.Values["returnvalue"])}, nil
}

func (c *Collector) checkHistoryProgress(ctx context.Context, sink HistoryStore, queue, key string, lastGap *string) error {
	cursor, err := sink.Cursor(ctx, queue)
	if err != nil || cursor == "" {
		return err
	}
	info, err := c.streamInfo(ctx, key)
	if err != nil {
		// BullMQ can create the queue's id key before its first event. An
		// initialized empty-stream cursor has no missing history to report.
		if cursor == "0-0" && strings.Contains(err.Error(), "no such key") {
			metrics.HistoryLag.WithLabelValues(queue).Set(0)
			return nil
		}
		return err
	}
	tail := info.LastGeneratedID
	cmp, err := store.CompareStreamIDs(cursor, tail)
	if err != nil {
		return err
	}
	if cmp >= 0 {
		metrics.HistoryLag.WithLabelValues(queue).Set(0)
	} else {
		ms, err := streamMillis(cursor)
		if err != nil {
			return err
		}
		metrics.HistoryLag.WithLabelValues(queue).Set(max(0, float64(time.Now().UnixMilli()-ms)/1000))
	}
	first := info.FirstEntry.ID
	gap := ""
	if first != "" {
		cmp, err := store.CompareStreamIDs(cursor, first)
		if err != nil {
			return err
		}
		if cmp < 0 && (info.EntriesAdded == 0 || info.EntriesAdded > info.Length) {
			gap = first
		}
	}
	// Redis 7+/Valkey expose the highest deleted ID; it also detects deletion
	// inside the retained range (XDEL), not just prefix trimming.
	if info.MaxDeletedEntryID != "" && info.MaxDeletedEntryID != "0-0" {
		cmp, err := store.CompareStreamIDs(cursor, info.MaxDeletedEntryID)
		if err != nil {
			return err
		}
		if cmp < 0 {
			gap = info.MaxDeletedEntryID
		}
	}
	if gap != "" && gap != *lastGap {
		metrics.HistoryGaps.WithLabelValues(queue).Inc()
		log.Printf("history stream gap queue=%q committed=%s unavailable_through=%s first_available=%s", queue, cursor, gap, first)
		*lastGap = gap
	}
	return nil
}
