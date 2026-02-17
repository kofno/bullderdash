package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kofno/bullderdash/internal/config"
	"github.com/kofno/bullderdash/internal/explorer"
	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/kofno/bullderdash/internal/web"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func withHTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if path, ok := normalizePath(r.URL.Path); ok {
			metrics.HTTPRequestDuration.WithLabelValues(
				r.Method,
				path,
				fmt.Sprintf("%d", rec.status),
			).Observe(time.Since(start).Seconds())
		}
	})
}

func normalizePath(path string) (string, bool) {
	switch {
	case path == "/":
		return "/", true
	case path == "/queues":
		return "/queues", true
	case path == "/queue/jobs":
		return "/queue/jobs", true
	case strings.HasPrefix(path, "/queue/"):
		return "/queue/:name", true
	case path == "/job/detail":
		return "/job/detail", true
	case path == "/metrics":
		return "/metrics", true
	case path == "/health" || path == "/healthz":
		return "/health", true
	case path == "/ready" || path == "/readyz":
		return "/ready", true
	default:
		return "", false
	}
}

func main() {
	// 1. Load configuration
	cfg := config.Load()
	log.Printf("🔧 Starting Bull-der-dash with config: Redis=%s, Port=%s, Prefix=%s, MetricsPoll=%ds",
		cfg.RedisAddr, cfg.ServerPort, cfg.QueuePrefix, cfg.MetricsPollSeconds)

	// 2. Setup Redis/Valkey client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func(rdb *redis.Client) {
		err := rdb.Close()
		if err != nil {
			log.Printf("⚠️ Failed to close Redis connection: %v", err)
		}
	}(rdb)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis/Valkey")

	exp := explorer.New(rdb)

	// 3. Setup HTTP routes
	mux := http.NewServeMux()

	// Main dashboard
	mux.HandleFunc("/", web.HomeHandler())

	mux.HandleFunc("/queues", web.DashboardHandler(exp, cfg.QueuePrefix))
	mux.HandleFunc("/queue/jobs", web.JobListHandler(exp))
	mux.HandleFunc("/queue/", web.QueueDetailHandler(exp))
	mux.HandleFunc("/job/detail", web.JobDetailHandler(exp))

	// Health checks (K8s friendly)
	mux.HandleFunc("/health", web.HealthHandler())
	mux.HandleFunc("/healthz", web.HealthHandler())
	mux.HandleFunc("/ready", web.ReadyHandler(exp))
	mux.HandleFunc("/readyz", web.ReadyHandler(exp))

	// Prometheus metrics
	mux.Handle("/metrics", promhttp.Handler())

	// Background queue stats poller for metrics freshness
	stopMetrics := make(chan struct{})
	go func() {
		pollSeconds := cfg.MetricsPollSeconds
		if pollSeconds < 1 {
			pollSeconds = 1
		}
		ticker := time.NewTicker(time.Duration(pollSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				queues, err := exp.DiscoverQueues(context.Background(), cfg.QueuePrefix)
				if err != nil {
					log.Printf("⚠️ DiscoverQueues (metrics poller) error: %v", err)
					continue
				}
				if _, err := exp.GetQueueStats(context.Background(), queues); err != nil {
					log.Printf("⚠️ GetQueueStats (metrics poller) error: %v", err)
				}
			case <-stopMetrics:
				return
			}
		}
	}()

	// 4. Setup server with graceful shutdown
	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      withHTTPMetrics(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("🚀 Bull-der-dash is running on http://localhost:%s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down gracefully...")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	close(stopMetrics)
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("👋 Server exited")
}
