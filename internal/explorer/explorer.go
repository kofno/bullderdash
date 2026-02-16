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

	return queues, nil
}

type QueueStats struct {
	Name      string
	Wait      int64
	Active    int64
	Failed    int64
	Completed int64
	Delayed   int64
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
}

func (e *Explorer) GetQueueStats(ctx context.Context, queues []string) ([]QueueStats, error) {
	start := time.Now()
	defer func() {
		metrics.RedisOperationDuration.WithLabelValues("get_queue_stats").Observe(time.Since(start).Seconds())
	}()

	pipe := e.client.Pipeline()

	// Create map to hold the command results
	type queueCmds struct {
		wait      *redis.IntCmd
		active    *redis.IntCmd
		failed    *redis.IntCmd
		completed *redis.IntCmd
		delayed   *redis.IntCmd
	}
	cmds := make(map[string]queueCmds)

	for _, q := range queues {
		cmds[q] = queueCmds{
			wait:      pipe.LLen(ctx, fmt.Sprintf("bull:%s:wait", q)),
			active:    pipe.LLen(ctx, fmt.Sprintf("bull:%s:active", q)),
			failed:    pipe.SCard(ctx, fmt.Sprintf("bull:%s:failed", q)),
			completed: pipe.SCard(ctx, fmt.Sprintf("bull:%s:completed", q)),
			delayed:   pipe.ZCard(ctx, fmt.Sprintf("bull:%s:delayed", q)),
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		metrics.RedisOperationErrors.WithLabelValues("get_queue_stats").Inc()
		return nil, err
	}

	var stats []QueueStats
	for _, q := range queues {
		stat := QueueStats{
			Name:      q,
			Wait:      cmds[q].wait.Val(),
			Active:    cmds[q].active.Val(),
			Failed:    cmds[q].failed.Val(),
			Completed: cmds[q].completed.Val(),
			Delayed:   cmds[q].delayed.Val(),
		}
		stats = append(stats, stat)

		// Update Prometheus metrics
		metrics.QueueWaiting.WithLabelValues(q).Set(float64(stat.Wait))
		metrics.QueueActive.WithLabelValues(q).Set(float64(stat.Active))
		metrics.QueueFailed.WithLabelValues(q).Set(float64(stat.Failed))
		metrics.QueueCompleted.WithLabelValues(q).Set(float64(stat.Completed))
		metrics.QueueDelayed.WithLabelValues(q).Set(float64(stat.Delayed))
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
		json.Unmarshal([]byte(dataStr), &job.Data)
	}
	if optsStr, ok := data["opts"]; ok {
		json.Unmarshal([]byte(optsStr), &job.Opts)
	}
	if progressStr, ok := data["progress"]; ok {
		json.Unmarshal([]byte(progressStr), &job.Progress)
	}
	if timestamp, ok := data["timestamp"]; ok {
		fmt.Sscanf(timestamp, "%d", &job.Timestamp)
	}
	if attemptsMade, ok := data["attemptsMade"]; ok {
		fmt.Sscanf(attemptsMade, "%d", &job.AttemptsMade)
	}
	if failedReason, ok := data["failedReason"]; ok {
		job.FailedReason = failedReason
	}
	if stacktrace, ok := data["stacktrace"]; ok {
		json.Unmarshal([]byte(stacktrace), &job.StackTrace)
	}
	if returnValue, ok := data["returnvalue"]; ok {
		json.Unmarshal([]byte(returnValue), &job.ReturnValue)
	}
	if finishedOn, ok := data["finishedOn"]; ok {
		fmt.Sscanf(finishedOn, "%d", &job.FinishedOn)
	}
	if processedOn, ok := data["processedOn"]; ok {
		fmt.Sscanf(processedOn, "%d", &job.ProcessedOn)
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

	// Check failed set
	if isMember, err := e.client.SIsMember(ctx, prefix+":failed", jobID).Result(); err == nil && isMember {
		return "failed"
	}

	// Check completed set
	if isMember, err := e.client.SIsMember(ctx, prefix+":completed", jobID).Result(); err == nil && isMember {
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
	case "failed":
		jobIDs, err = e.client.SMembers(ctx, prefix+":failed").Result()
		if len(jobIDs) > limit {
			jobIDs = jobIDs[:limit]
		}
	case "completed":
		jobIDs, err = e.client.SMembers(ctx, prefix+":completed").Result()
		if len(jobIDs) > limit {
			jobIDs = jobIDs[:limit]
		}
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
		summaries = append(summaries, JobSummary{
			ID:           job.ID,
			Name:         job.Name,
			State:        state,
			Queue:        queueName,
			Timestamp:    time.Unix(job.Timestamp/1000, 0),
			AttemptsMade: job.AttemptsMade,
		})
	}

	return summaries, nil
}
