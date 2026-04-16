package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Queue metrics
	QueueWaiting = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_waiting",
			Help: "Number of jobs waiting in queue",
		},
		[]string{"queue"},
	)

	QueueActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_active",
			Help: "Number of jobs currently being processed",
		},
		[]string{"queue"},
	)

	QueuePaused = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_paused",
			Help: "Number of jobs paused in queue",
		},
		[]string{"queue"},
	)

	QueuePrioritized = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_prioritized",
			Help: "Number of prioritized jobs in queue",
		},
		[]string{"queue"},
	)

	QueueWaitingChildren = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_waiting_children",
			Help: "Number of jobs waiting on children in queue",
		},
		[]string{"queue"},
	)

	QueueFailed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_failed",
			Help: "Number of failed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueCompleted = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_completed",
			Help: "Number of completed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueDelayed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_delayed",
			Help: "Number of delayed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueStalled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_stalled",
			Help: "Number of stalled jobs in queue",
		},
		[]string{"queue"},
	)

	QueueOrphaned = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_orphaned",
			Help: "Number of orphaned job hashes not in any state list",
		},
		[]string{"queue"},
	)

	// Workload metrics are derived from BullMQ event streams in a background
	// collector. They intentionally do not perform Redis work during /metrics.
	WorkloadJobsFinished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bullmq_jobs_finished_total",
			Help: "Total number of BullMQ jobs observed finishing",
		},
		[]string{"queue", "name", "result"},
	)

	WorkloadJobCompletionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "bullmq_job_completion_duration_seconds",
			Help: "Observed BullMQ job processing duration from processedOn to finishedOn",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.075,
				0.1, 0.15, 0.25, 0.5, 0.75,
				1, 1.5, 2, 2.5, 3,
				4, 5, 7.5, 10, 15,
				30, 60, 120, 300,
			},
		},
		[]string{"queue", "name", "result"},
	)

	WorkloadEventLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_workload_event_lag_seconds",
			Help: "Approximate age of the latest BullMQ event stream entry observed by the workload metrics collector",
		},
		[]string{"queue"},
	)

	WorkloadEventsRead = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bullmq_workload_events_read_total",
			Help: "Total number of BullMQ event stream entries read by the workload metrics collector",
		},
		[]string{"queue", "event"},
	)

	WorkloadEventsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bullmq_workload_events_dropped_total",
			Help: "Total number of BullMQ terminal events dropped by the workload metrics collector",
		},
		[]string{"queue", "reason"},
	)

	WorkloadJobLookupErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bullmq_workload_job_lookup_errors_total",
			Help: "Total number of job hash lookup errors from the workload metrics collector",
		},
		[]string{"queue", "reason"},
	)

	// HTTP metrics
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// Explorer metrics
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operation_duration_seconds",
			Help:    "Redis operation latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	RedisOperationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_operation_errors_total",
			Help: "Total number of Redis operation errors",
		},
		[]string{"operation"},
	)
)
