package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/redis/go-redis/v9"
)

type Explorer struct {
	client *redis.Client
}

func New(client *redis.Client) *Explorer {
	return &Explorer{client: client}
}

// DiscoverQueues finds all BullMQ queues by looking for the ":id" suffix
func (e *Explorer) DiscoverQueues(ctx context.Context, prefix string) ([]string, error) {
	var cursor uint64
	seen := make(map[string]struct{})

	// Default prefix is usually "bull"
	pattern := prefix + ":*:id"

	for {
		keys, nextCursor, err := e.client.Scan(ctx, cursor, pattern, 10).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			queueName, ok := strings.CutPrefix(key, prefix+":")
			if !ok {
				continue
			}
			queueName, ok = strings.CutSuffix(queueName, ":id")
			if !ok || queueName == "" {
				continue
			}
			seen[queueName] = struct{}{}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// Ensure we never return nil slice, return empty slice instead
	queues := make([]string, 0, len(seen))
	for queue := range seen {
		queues = append(queues, queue)
	}
	sort.Strings(queues)
	return queues, nil
}

type QueueStats struct {
	Name            string
	Wait            int64
	Active          int64
	Paused          int64
	Prioritized     int64
	WaitingChildren int64
	Failed          int64
	Completed       int64
	Delayed         int64
	Stalled         int64
	Orphaned        int64
	OrphanedKnown   bool
	Total           int64
}

// Job represents a BullMQ job
type Job struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Data         map[string]interface{} `json:"data"`
	Opts         map[string]interface{} `json:"opts"`
	Progress     interface{}            `json:"progress"`
	Delay        int64                  `json:"delay"`
	Timestamp    int64                  `json:"timestamp"`
	AttemptsMade int                    `json:"attemptsMade"`
	FailedReason string                 `json:"failedReason"`
	StackTrace   []string               `json:"stacktrace"`
	ReturnValue  interface{}            `json:"returnvalue"`
	FinishedOn   int64                  `json:"finishedOn"`
	ProcessedOn  int64                  `json:"processedOn"`
	State        string                 `json:"-"` // We'll set this based on which list it's in
	Queue        string                 `json:"-"` // Queue name
}

// JobSummary is a lighter weight version for list views
type JobSummary struct {
	ID           string
	Name         string
	State        string
	Queue        string
	Timestamp    time.Time
	AttemptsMade int
	Data         string
	Opts         string
	FailedReason string
}

func (e *Explorer) GetQueueStats(ctx context.Context, queues []string) ([]QueueStats, error) {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_queue_stats").Observe(time.Since(start).Seconds())
	}()

	var stats []QueueStats

	// Get stats for each queue individually to handle errors per-queue
	for _, q := range queues {
		prefix := fmt.Sprintf("bull:%s", q)

		// Use individual commands instead of pipeline to handle per-queue errors
		waitLen, _ := e.client.LLen(ctx, prefix+":wait").Result()
		activeLen, _ := e.client.LLen(ctx, prefix+":active").Result()
		pausedLen, _ := e.client.LLen(ctx, prefix+":paused").Result()
		prioritizedLen, _ := e.client.ZCard(ctx, prefix+":prioritized").Result()
		waitingChildrenLen, _ := e.client.ZCard(ctx, prefix+":waiting-children").Result()

		failedCard, _ := e.client.ZCard(ctx, prefix+":failed").Result()
		completedCard, _ := e.client.ZCard(ctx, prefix+":completed").Result()
		delayedCard, _ := e.client.ZCard(ctx, prefix+":delayed").Result()
		stalledCard, _ := e.client.ZCard(ctx, prefix+":stalled").Result()

		// Count total job hashes (all keys matching the job ID pattern)
		var totalJobHashes int64
		cursor := uint64(0)
		jobIDsInQueues := make(map[string]bool)

		// First, collect all job IDs that are in state lists
		if waitIDs, err := e.client.LRange(ctx, prefix+":wait", 0, -1).Result(); err == nil {
			for _, id := range waitIDs {
				jobIDsInQueues[id] = true
			}
		}
		if activeIDs, err := e.client.LRange(ctx, prefix+":active", 0, -1).Result(); err == nil {
			for _, id := range activeIDs {
				jobIDsInQueues[id] = true
			}
		}
		if pausedIDs, err := e.client.LRange(ctx, prefix+":paused", 0, -1).Result(); err == nil {
			for _, id := range pausedIDs {
				jobIDsInQueues[id] = true
			}
		}
		if prioritizedIDs, err := e.client.ZRange(ctx, prefix+":prioritized", 0, -1).Result(); err == nil {
			for _, id := range prioritizedIDs {
				jobIDsInQueues[id] = true
			}
		}
		if waitingChildrenIDs, err := e.client.ZRange(ctx, prefix+":waiting-children", 0, -1).Result(); err == nil {
			for _, id := range waitingChildrenIDs {
				jobIDsInQueues[id] = true
			}
		}
		if failedIDs, err := e.client.ZRange(ctx, prefix+":failed", 0, -1).Result(); err == nil {
			for _, id := range failedIDs {
				jobIDsInQueues[id] = true
			}
		}
		if completedIDs, err := e.client.ZRange(ctx, prefix+":completed", 0, -1).Result(); err == nil {
			for _, id := range completedIDs {
				jobIDsInQueues[id] = true
			}
		}
		if delayedResults, err := e.client.ZRangeWithScores(ctx, prefix+":delayed", 0, -1).Result(); err == nil {
			for _, z := range delayedResults {
				if id, ok := z.Member.(string); ok {
					jobIDsInQueues[id] = true
				}
			}
		}
		if stalledResults, err := e.client.ZRangeWithScores(ctx, prefix+":stalled", 0, -1).Result(); err == nil {
			for _, z := range stalledResults {
				if id, ok := z.Member.(string); ok {
					jobIDsInQueues[id] = true
				}
			}
		}

		// Now scan for all job hash keys (exclude metadata keys and ensure hash has "name")
		for {
			keys, nextCursor, err := e.client.Scan(ctx, cursor, prefix+":*", 100).Result()
			if err != nil {
				break
			}

			for _, key := range keys {
				// Extract the suffix after the queue name
				suffix := strings.TrimPrefix(key, prefix+":")

				// Skip metadata keys and state list keys
				if suffix == "id" || suffix == "meta" || suffix == "events" ||
					suffix == "wait" || suffix == "active" || suffix == "failed" ||
					suffix == "completed" || suffix == "delayed" || suffix == "stalled" ||
					suffix == "paused" || suffix == "priority" || suffix == "prioritized" ||
					suffix == "waiting-children" {
					continue
				}

				keyType, err := e.client.Type(ctx, key).Result()
				if err != nil || keyType != "hash" {
					continue
				}
				isJobHash, err := e.client.HExists(ctx, key, "name").Result()
				if err != nil || !isJobHash {
					continue
				}

				totalJobHashes++
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}

		// Calculate orphaned jobs (job hashes not in any state list)
		orphanedCount := totalJobHashes - int64(len(jobIDsInQueues))
		if orphanedCount < 0 {
			orphanedCount = 0
		}

		stat := QueueStats{
			Name:            q,
			Wait:            waitLen,
			Active:          activeLen,
			Paused:          pausedLen,
			Prioritized:     prioritizedLen,
			WaitingChildren: waitingChildrenLen,
			Failed:          failedCard,
			Completed:       completedCard,
			Delayed:         delayedCard,
			Stalled:         stalledCard,
			Orphaned:        orphanedCount,
			OrphanedKnown:   true,
			Total:           waitLen + activeLen + pausedLen + prioritizedLen + waitingChildrenLen + failedCard + completedCard + delayedCard + stalledCard + orphanedCount,
		}
		stats = append(stats, stat)

		updateQueueMetrics(stat)
	}

	// Ensure we never return nil slice, return empty slice instead
	if stats == nil {
		stats = make([]QueueStats, 0)
	}
	return stats, nil
}

// Ping verifies Redis/Valkey connectivity with a cheap readiness-safe command.
func (e *Explorer) Ping(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("ping").Observe(time.Since(start).Seconds())
	}()

	if err := e.client.Ping(ctx).Err(); err != nil {
		metrics.RedisOperationErrors.WithLabelValues("ping").Inc()
		return err
	}
	return nil
}

// GetQueueStatsFast returns queue counts using only cheap cardinality operations.
func (e *Explorer) GetQueueStatsFast(ctx context.Context, queuePrefix string, queues []string) ([]QueueStats, error) {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_queue_stats_fast").Observe(time.Since(start).Seconds())
	}()

	if len(queues) == 0 {
		return make([]QueueStats, 0), nil
	}

	type queueCommands struct {
		wait            *redis.IntCmd
		active          *redis.IntCmd
		paused          *redis.IntCmd
		prioritized     *redis.IntCmd
		waitingChildren *redis.IntCmd
		failed          *redis.IntCmd
		completed       *redis.IntCmd
		delayed         *redis.IntCmd
		stalled         *redis.IntCmd
	}

	cmds := make([]queueCommands, len(queues))
	pipe := e.client.Pipeline()
	for i, q := range queues {
		prefix := fmt.Sprintf("%s:%s", queuePrefix, q)
		cmds[i] = queueCommands{
			wait:            pipe.LLen(ctx, prefix+":wait"),
			active:          pipe.LLen(ctx, prefix+":active"),
			paused:          pipe.LLen(ctx, prefix+":paused"),
			prioritized:     pipe.ZCard(ctx, prefix+":prioritized"),
			waitingChildren: pipe.ZCard(ctx, prefix+":waiting-children"),
			failed:          pipe.ZCard(ctx, prefix+":failed"),
			completed:       pipe.ZCard(ctx, prefix+":completed"),
			delayed:         pipe.ZCard(ctx, prefix+":delayed"),
			stalled:         pipe.ZCard(ctx, prefix+":stalled"),
		}
	}

	_, _ = pipe.Exec(ctx)

	stats := make([]QueueStats, 0, len(queues))
	for i, q := range queues {
		waitLen, err := intCmdValue(cmds[i].wait)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		activeLen, err := intCmdValue(cmds[i].active)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		pausedLen, err := intCmdValue(cmds[i].paused)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		prioritizedLen, err := intCmdValue(cmds[i].prioritized)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		waitingChildrenLen, err := intCmdValue(cmds[i].waitingChildren)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		failedLen, err := intCmdValue(cmds[i].failed)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		completedLen, err := intCmdValue(cmds[i].completed)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		delayedLen, err := intCmdValue(cmds[i].delayed)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}
		stalledLen, err := intCmdValue(cmds[i].stalled)
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_queue_stats_fast").Inc()
			return nil, err
		}

		stat := QueueStats{
			Name:            q,
			Wait:            waitLen,
			Active:          activeLen,
			Paused:          pausedLen,
			Prioritized:     prioritizedLen,
			WaitingChildren: waitingChildrenLen,
			Failed:          failedLen,
			Completed:       completedLen,
			Delayed:         delayedLen,
			Stalled:         stalledLen,
			OrphanedKnown:   false,
			Total:           waitLen + activeLen + pausedLen + prioritizedLen + waitingChildrenLen + failedLen + completedLen + delayedLen + stalledLen,
		}
		stats = append(stats, stat)
		updateQueueMetrics(stat)
	}

	return stats, nil
}

// GetJob retrieves a single job by ID from a queue
func (e *Explorer) GetJob(ctx context.Context, queueName, jobID string) (*Job, error) {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_job").Observe(time.Since(start).Seconds())
	}()

	key := fmt.Sprintf("bull:%s:%s", queueName, jobID)
	data, err := e.client.HGetAll(ctx, key).Result()
	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("get_job").Inc()
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	job := &Job{
		ID:    jobID,
		Queue: queueName,
	}

	// Parse the JSON fields
	if name, ok := data["name"]; ok {
		job.Name = name
	}
	if dataStr, ok := data["data"]; ok {
		err := json.Unmarshal([]byte(dataStr), &job.Data)
		if err != nil {
			return nil, err
		}
	}
	if optsStr, ok := data["opts"]; ok {
		err := json.Unmarshal([]byte(optsStr), &job.Opts)
		if err != nil {
			return nil, err
		}
	}
	if progressStr, ok := data["progress"]; ok {
		err := json.Unmarshal([]byte(progressStr), &job.Progress)
		if err != nil {
			return nil, err
		}
	}
	if timestamp, ok := data["timestamp"]; ok {
		_, err := fmt.Sscanf(timestamp, "%d", &job.Timestamp)
		if err != nil {
			return nil, err
		}
	}
	if attemptsMade, ok := data["attemptsMade"]; ok {
		_, err := fmt.Sscanf(attemptsMade, "%d", &job.AttemptsMade)
		if err != nil {
			return nil, err
		}
	}
	if failedReason, ok := data["failedReason"]; ok {
		job.FailedReason = failedReason
	}
	if stacktrace, ok := data["stacktrace"]; ok {
		err := json.Unmarshal([]byte(stacktrace), &job.StackTrace)
		if err != nil {
			return nil, err
		}
	}
	if returnValue, ok := data["returnvalue"]; ok {
		err := json.Unmarshal([]byte(returnValue), &job.ReturnValue)
		if err != nil {
			return nil, err
		}
	}
	if finishedOn, ok := data["finishedOn"]; ok {
		_, err := fmt.Sscanf(finishedOn, "%d", &job.FinishedOn)
		if err != nil {
			return nil, err
		}
	}
	if processedOn, ok := data["processedOn"]; ok {
		_, err := fmt.Sscanf(processedOn, "%d", &job.ProcessedOn)
		if err != nil {
			return nil, err
		}
	}

	// Determine job state by checking which list/set it's in
	job.State = e.determineJobState(ctx, queueName, jobID)

	return job, nil
}

// determineJobState checks which list/set a job belongs to
func (e *Explorer) determineJobState(ctx context.Context, queueName, jobID string) string {
	prefix := fmt.Sprintf("bull:%s", queueName)

	// Check active list
	if _, err := e.client.LPos(ctx, prefix+":active", jobID, redis.LPosArgs{}).Result(); err == nil {
		return "active"
	}

	// Check waiting list
	if _, err := e.client.LPos(ctx, prefix+":wait", jobID, redis.LPosArgs{}).Result(); err == nil {
		return "waiting"
	}

	// Check paused list
	if _, err := e.client.LPos(ctx, prefix+":paused", jobID, redis.LPosArgs{}).Result(); err == nil {
		return "paused"
	}

	// Check prioritized zset
	if _, err := e.client.ZScore(ctx, prefix+":prioritized", jobID).Result(); err == nil {
		return "prioritized"
	}

	// Check waiting-children zset
	if _, err := e.client.ZScore(ctx, prefix+":waiting-children", jobID).Result(); err == nil {
		return "waiting-children"
	}

	// Check failed set
	if _, err := e.client.ZScore(ctx, prefix+":failed", jobID).Result(); err == nil {
		return "failed"
	}

	// Check completed set
	if _, err := e.client.ZScore(ctx, prefix+":completed", jobID).Result(); err == nil {
		return "completed"
	}

	// Check delayed sorted set
	if _, err := e.client.ZScore(ctx, prefix+":delayed", jobID).Result(); err == nil {
		return "delayed"
	}

	return "unknown"
}

// GetJobsByState retrieves jobs in a specific state (waiting, active, failed, etc.)
func (e *Explorer) GetJobsByState(ctx context.Context, queueName, state string, limit int) ([]JobSummary, error) {
	return e.GetJobsByStatePage(ctx, queueName, state, 0, limit)
}

// GetJobsByStatePage retrieves jobs in a specific state with offset/limit pagination.
func (e *Explorer) GetJobsByStatePage(ctx context.Context, queueName, state string, offset, limit int) ([]JobSummary, error) {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_jobs_by_state").Observe(time.Since(start).Seconds())
	}()

	if limit <= 0 {
		return make([]JobSummary, 0), nil
	}
	if offset < 0 {
		offset = 0
	}

	prefix := fmt.Sprintf("bull:%s", queueName)
	var jobIDs []string
	var err error
	startIdx := int64(offset)
	endIdx := int64(offset + limit - 1)

	switch state {
	case "waiting":
		jobIDs, err = e.client.LRange(ctx, prefix+":wait", startIdx, endIdx).Result()
	case "active":
		jobIDs, err = e.client.LRange(ctx, prefix+":active", startIdx, endIdx).Result()
	case "paused":
		jobIDs, err = e.client.LRange(ctx, prefix+":paused", startIdx, endIdx).Result()
	case "prioritized":
		jobIDs, err = e.client.ZRange(ctx, prefix+":prioritized", startIdx, endIdx).Result()
	case "waiting-children":
		jobIDs, err = e.client.ZRange(ctx, prefix+":waiting-children", startIdx, endIdx).Result()
	case "failed":
		jobIDs, err = e.client.ZRange(ctx, prefix+":failed", startIdx, endIdx).Result()
	case "completed":
		jobIDs, err = e.client.ZRange(ctx, prefix+":completed", startIdx, endIdx).Result()
	case "delayed":
		// For delayed jobs, we get them from the sorted set
		results, err := e.client.ZRangeWithScores(ctx, prefix+":delayed", startIdx, endIdx).Result()
		if err != nil {
			metrics.RedisOperationErrors.WithLabelValues("get_jobs_by_state").Inc()
			return nil, err
		}
		for _, z := range results {
			if id, ok := z.Member.(string); ok {
				jobIDs = append(jobIDs, id)
			}
		}
	case "stalled":
		jobIDs, err = e.client.ZRange(ctx, prefix+":stalled", startIdx, endIdx).Result()
	default:
		return nil, fmt.Errorf("unknown state: %s", state)
	}

	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("get_jobs_by_state").Inc()
		return nil, err
	}

	return e.loadJobSummaries(ctx, queueName, state, jobIDs)
}

// GetJobsAcrossStates retrieves jobs from all known states for a queue.
func (e *Explorer) GetJobsAcrossStates(ctx context.Context, queueName string, limitPerState int) ([]JobSummary, error) {
	return e.GetJobsAcrossStatesPage(ctx, queueName, 0, limitPerState)
}

// GetJobsAcrossStatesPage retrieves jobs from all known states for a queue with offset/limit pagination per state.
func (e *Explorer) GetJobsAcrossStatesPage(ctx context.Context, queueName string, offsetPerState, limitPerState int) ([]JobSummary, error) {
	states := []string{
		"waiting",
		"active",
		"paused",
		"prioritized",
		"waiting-children",
		"failed",
		"completed",
		"delayed",
		"stalled",
	}

	seen := make(map[string]bool)
	var summaries []JobSummary
	for _, state := range states {
		jobs, err := e.GetJobsByStatePage(ctx, queueName, state, offsetPerState, limitPerState)
		if err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if seen[job.ID] {
				continue
			}
			seen[job.ID] = true
			summaries = append(summaries, job)
		}
	}

	return summaries, nil
}

func intCmdValue(cmd *redis.IntCmd) (int64, error) {
	if err := cmd.Err(); err != nil && !isBenignCountError(err) {
		return 0, err
	}
	return cmd.Val(), nil
}

func isBenignCountError(err error) bool {
	if err == nil || err == redis.Nil {
		return true
	}
	return strings.Contains(err.Error(), "WRONGTYPE")
}

func updateQueueMetrics(stat QueueStats) {
	metrics.QueueWaiting.WithLabelValues(stat.Name).Set(float64(stat.Wait))
	metrics.QueueActive.WithLabelValues(stat.Name).Set(float64(stat.Active))
	metrics.QueuePaused.WithLabelValues(stat.Name).Set(float64(stat.Paused))
	metrics.QueuePrioritized.WithLabelValues(stat.Name).Set(float64(stat.Prioritized))
	metrics.QueueWaitingChildren.WithLabelValues(stat.Name).Set(float64(stat.WaitingChildren))
	metrics.QueueFailed.WithLabelValues(stat.Name).Set(float64(stat.Failed))
	metrics.QueueCompleted.WithLabelValues(stat.Name).Set(float64(stat.Completed))
	metrics.QueueDelayed.WithLabelValues(stat.Name).Set(float64(stat.Delayed))
	metrics.QueueStalled.WithLabelValues(stat.Name).Set(float64(stat.Stalled))
	if stat.OrphanedKnown {
		metrics.QueueOrphaned.WithLabelValues(stat.Name).Set(float64(stat.Orphaned))
	}
}

func (e *Explorer) loadJobSummaries(ctx context.Context, queueName, state string, jobIDs []string) ([]JobSummary, error) {
	if len(jobIDs) == 0 {
		return make([]JobSummary, 0), nil
	}

	pipe := e.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		key := fmt.Sprintf("bull:%s:%s", queueName, jobID)
		cmds = append(cmds, pipe.HGetAll(ctx, key))
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		metrics.RedisOperationErrors.WithLabelValues("get_jobs_by_state").Inc()
		return nil, err
	}

	summaries := make([]JobSummary, 0, len(jobIDs))
	for idx, cmd := range cmds {
		if err := cmd.Err(); err != nil && err != redis.Nil {
			continue
		}

		data := cmd.Val()
		if len(data) == 0 {
			continue
		}

		jobID := jobIDs[idx]
		summary := JobSummary{
			ID:    jobID,
			Name:  data["name"],
			State: state,
			Queue: queueName,
		}

		if timestamp := data["timestamp"]; timestamp != "" {
			var ts int64
			if _, err := fmt.Sscanf(timestamp, "%d", &ts); err == nil && ts > 0 {
				summary.Timestamp = time.Unix(ts/1000, 0)
			}
		}
		if attemptsMade := data["attemptsMade"]; attemptsMade != "" {
			_, _ = fmt.Sscanf(attemptsMade, "%d", &summary.AttemptsMade)
		}
		if failedReason, ok := data["failedReason"]; ok {
			summary.FailedReason = failedReason
		}
		if dataStr, ok := data["data"]; ok {
			summary.Data = compactJSON(dataStr)
		}
		if optsStr, ok := data["opts"]; ok {
			summary.Opts = compactJSON(optsStr)
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func compactJSON(raw string) string {
	if raw == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return raw
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		return raw
	}
	return string(encoded)
}
