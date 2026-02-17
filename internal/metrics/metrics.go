package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Queue metrics
	QueueWaiting = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_waiting_total",
			Help: "Number of jobs waiting in queue",
		},
		[]string{"queue"},
	)

	QueueActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_active_total",
			Help: "Number of jobs currently being processed",
		},
		[]string{"queue"},
	)

	QueueFailed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_failed_total",
			Help: "Number of failed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueCompleted = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_completed_total",
			Help: "Number of completed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueDelayed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_delayed_total",
			Help: "Number of delayed jobs in queue",
		},
		[]string{"queue"},
	)

	QueueStalled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_stalled_total",
			Help: "Number of stalled jobs in queue",
		},
		[]string{"queue"},
	)

	QueueOrphaned = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bullmq_queue_orphaned_total",
			Help: "Number of orphaned job hashes not in any state list",
		},
		[]string{"queue"},
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
