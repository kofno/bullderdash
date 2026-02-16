package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
	"github.com/kofno/bullderdash/internal/metrics"
)

// The template for our queue list
const queueListTmpl = `
<table class="min-w-full divide-y divide-gray-200">
    <thead class="bg-gray-50">
        <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Queue</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Waiting</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Active</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Failed</th>
        </tr>
    </thead>
    <tbody class="bg-white divide-y divide-gray-200">
        {{range .}}
        <tr>
            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">{{.Name}}</td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 text-center">{{.Wait}}</td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 text-center text-blue-600 font-bold">{{.Active}}</td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center">
                <span class="px-2 py-1 rounded text-xs {{if gt .Failed 0}}bg-red-100 text-red-800 font-bold{{else}}bg-gray-100 text-gray-400{{end}}">
                    {{.Failed}}
                </span>
            </td>
        </tr>
        {{end}}
    </tbody>
</table>
`

func DashboardHandler(exp *explorer.Explorer, prefix string) http.HandlerFunc {
	tmpl := template.Must(template.New("queues").Parse(queueListTmpl))

	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			duration := time.Since(start).Seconds()
			status := "200"
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
		}()

		queues, err := exp.DiscoverQueues(r.Context(), prefix)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		queueStats, err := exp.GetQueueStats(r.Context(), queues)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = tmpl.Execute(w, queueStats)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// JobListHandler shows jobs in a specific state for a queue
func JobListHandler(exp *explorer.Explorer) http.HandlerFunc {
	tmpl := template.Must(template.New("jobs").Parse(jobListTmpl))

	return func(w http.ResponseWriter, r *http.Request) {
		queueName := r.URL.Query().Get("queue")
		state := r.URL.Query().Get("state")
		if queueName == "" || state == "" {
			http.Error(w, "queue and state parameters required", http.StatusBadRequest)
			return
		}

		jobs, err := exp.GetJobsByState(r.Context(), queueName, state, 100)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			Queue string
			State string
			Jobs  []explorer.JobSummary
		}{
			Queue: queueName,
			State: state,
			Jobs:  jobs,
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// JobDetailHandler shows details for a specific job
func JobDetailHandler(exp *explorer.Explorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queueName := r.URL.Query().Get("queue")
		jobID := r.URL.Query().Get("id")
		if queueName == "" || jobID == "" {
			http.Error(w, "queue and id parameters required", http.StatusBadRequest)
			return
		}

		job, err := exp.GetJob(r.Context(), queueName, jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return JSON for now - we can add HTML template later
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(job)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// HealthHandler provides health check endpoint
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprint(w, "OK")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// ReadyHandler provides readiness check endpoint
func ReadyHandler(exp *explorer.Explorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to ping Redis
		_, err := exp.DiscoverQueues(r.Context(), "bull")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Redis unavailable: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err = fmt.Fprint(w, "Ready")
		if err != nil {
			return
		}
	}
}

const jobListTmpl = `
<div class="space-y-4">
    <div class="flex justify-between items-center mb-4">
        <h2 class="text-xl font-bold text-gray-800">
            {{.Queue}} - {{.State}} Jobs ({{len .Jobs}})
        </h2>
        <a href="/" class="text-indigo-600 hover:text-indigo-800">← Back to Queues</a>
    </div>
    {{if .Jobs}}
    <table class="min-w-full divide-y divide-gray-200">
        <thead class="bg-gray-50">
            <tr>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Attempts</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
        </thead>
        <tbody class="bg-white divide-y divide-gray-200">
            {{range .Jobs}}
            <tr class="hover:bg-gray-50">
                <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                <td class="px-6 py-4 text-sm text-gray-500">{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
                <td class="px-6 py-4 text-sm text-gray-500">{{.AttemptsMade}}</td>
                <td class="px-6 py-4 text-sm">
                    <a href="/job/detail?queue={{.Queue}}&id={{.ID}}" 
                       class="text-indigo-600 hover:text-indigo-900"
                       target="_blank">
                        View Details →
                    </a>
                </td>
            </tr>
            {{end}}
        </tbody>
    </table>
    {{else}}
    <div class="text-center py-8 text-gray-500">
        No jobs in {{.State}} state
    </div>
    {{end}}
</div>
`
