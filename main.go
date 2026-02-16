package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kofno/bullderdash/internal/config"
	"github.com/kofno/bullderdash/internal/explorer"
	"github.com/kofno/bullderdash/internal/web"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Load configuration
	cfg := config.Load()
	log.Printf("🔧 Starting Bull-der-dash with config: Redis=%s, Port=%s, Prefix=%s",
		cfg.RedisAddr, cfg.ServerPort, cfg.QueuePrefix)

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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprint(w, `
        <html>
            <head>
                <script src="https://unpkg.com/htmx.org@1.9.10"></script>
                <script src="https://cdn.tailwindcss.com"></script>
                <title>Bull-der-dash</title>
            </head>
            <body class="bg-gray-50 p-10">
                <div class="max-w-6xl mx-auto bg-white shadow rounded-lg p-6">
                    <div class="flex justify-between items-center mb-6">
                        <h1 class="text-2xl font-bold text-indigo-600">🐂 Bull-der-dash Explorer</h1>
                        <div class="flex gap-4 text-sm text-gray-600">
                            <a href="/metrics" target="_blank" class="hover:text-indigo-600">📊 Metrics</a>
                            <a href="/health" target="_blank" class="hover:text-indigo-600">💚 Health</a>
                        </div>
                    </div>
                    <div id="queue-list" hx-get="/queues" hx-trigger="load, every 5s">
                        Loading queues...
                    </div>
                </div>
            </body>
        </html>
        `)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("/queues", web.DashboardHandler(exp, cfg.QueuePrefix))
	mux.HandleFunc("/queue/jobs", web.JobListHandler(exp))
	mux.HandleFunc("/job/detail", web.JobDetailHandler(exp))

	// Health checks (K8s friendly)
	mux.HandleFunc("/health", web.HealthHandler())
	mux.HandleFunc("/healthz", web.HealthHandler())
	mux.HandleFunc("/ready", web.ReadyHandler(exp))
	mux.HandleFunc("/readyz", web.ReadyHandler(exp))

	// Prometheus metrics
	mux.Handle("/metrics", promhttp.Handler())

	// 4. Setup server with graceful shutdown
	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      mux,
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

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("👋 Server exited")
}
