package explorer

import (
	"context"
	"encoding/json"
	"fmt"
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
	var queues []string

	// Default prefix is usually "bull"
	pattern := prefix + ":*:id"

	for {
		keys, nextCursor, err := e.client.Scan(ctx, cursor, pattern, 10).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			// Key format: "bull:my-queue-name:id"
			parts := strings.Split(key, ":")
			if len(parts) >= 3 {
				queues = append(queues, parts[1])
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// Ensure we never return nil slice, return empty slice instead
	if queues == nil {
		queues = make([]string, 0)
	}
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
			Total:           waitLen + activeLen + pausedLen + prioritizedLen + waitingChildrenLen + failedCard + completedCard + delayedCard + stalledCard + orphanedCount,
		}
		stats = append(stats, stat)

		// Update Prometheus metrics
		metrics.QueueWaiting.WithLabelValues(q).Set(float64(stat.Wait))
		metrics.QueueActive.WithLabelValues(q).Set(float64(stat.Active))
		metrics.QueuePaused.WithLabelValues(q).Set(float64(stat.Paused))
		metrics.QueuePrioritized.WithLabelValues(q).Set(float64(stat.Prioritized))
		metrics.QueueWaitingChildren.WithLabelValues(q).Set(float64(stat.WaitingChildren))
		metrics.QueueFailed.WithLabelValues(q).Set(float64(stat.Failed))
		metrics.QueueCompleted.WithLabelValues(q).Set(float64(stat.Completed))
		metrics.QueueDelayed.WithLabelValues(q).Set(float64(stat.Delayed))
		metrics.QueueStalled.WithLabelValues(q).Set(float64(stat.Stalled))
		metrics.QueueOrphaned.WithLabelValues(q).Set(float64(stat.Orphaned))
	}

	// Ensure we never return nil slice, return empty slice instead
	if stats == nil {
		stats = make([]QueueStats, 0)
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
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_jobs_by_state").Observe(time.Since(start).Seconds())
	}()

	prefix := fmt.Sprintf("bull:%s", queueName)
	var jobIDs []string
	var err error

	switch state {
	case "waiting":
		jobIDs, err = e.client.LRange(ctx, prefix+":wait", 0, int64(limit-1)).Result()
	case "active":
		jobIDs, err = e.client.LRange(ctx, prefix+":active", 0, int64(limit-1)).Result()
	case "paused":
		jobIDs, err = e.client.LRange(ctx, prefix+":paused", 0, int64(limit-1)).Result()
	case "prioritized":
		jobIDs, err = e.client.ZRange(ctx, prefix+":prioritized", 0, int64(limit-1)).Result()
	case "waiting-children":
		jobIDs, err = e.client.ZRange(ctx, prefix+":waiting-children", 0, int64(limit-1)).Result()
	case "failed":
		jobIDs, err = e.client.ZRange(ctx, prefix+":failed", 0, int64(limit-1)).Result()
	case "completed":
		jobIDs, err = e.client.ZRange(ctx, prefix+":completed", 0, int64(limit-1)).Result()
	case "delayed":
		// For delayed jobs, we get them from the sorted set
		results, err := e.client.ZRangeWithScores(ctx, prefix+":delayed", 0, int64(limit-1)).Result()
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
		jobIDs, err = e.client.ZRange(ctx, prefix+":stalled", 0, int64(limit-1)).Result()
	default:
		return nil, fmt.Errorf("unknown state: %s", state)
	}

	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("get_jobs_by_state").Inc()
		return nil, err
	}

	// Fetch basic info for each job
	var summaries []JobSummary
	for _, jobID := range jobIDs {
		job, err := e.GetJob(ctx, queueName, jobID)
		if err != nil {
			continue // Skip jobs that can't be loaded
		}
		summaryState := state
		if job.State != "" && job.State != "unknown" {
			summaryState = job.State
		}
		dataStr := ""
		if job.Data != nil {
			if b, err := json.Marshal(job.Data); err == nil {
				dataStr = string(b)
			}
		}
		optsStr := ""
		if job.Opts != nil {
			if b, err := json.Marshal(job.Opts); err == nil {
				optsStr = string(b)
			}
		}
		summaries = append(summaries, JobSummary{
			ID:           job.ID,
			Name:         job.Name,
			State:        summaryState,
			Queue:        queueName,
			Timestamp:    time.Unix(job.Timestamp/1000, 0),
			AttemptsMade: job.AttemptsMade,
			Data:         dataStr,
			Opts:         optsStr,
			FailedReason: job.FailedReason,
		})
	}

	return summaries, nil
}

// GetJobsAcrossStates retrieves jobs from all known states for a queue.
func (e *Explorer) GetJobsAcrossStates(ctx context.Context, queueName string, limitPerState int) ([]JobSummary, error) {
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
		jobs, err := e.GetJobsByState(ctx, queueName, state, limitPerState)
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
