package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
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
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Completed</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Failed</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Delayed</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Actions</th>
        </tr>
    </thead>
    <tbody class="bg-white divide-y divide-gray-200">
        {{range .}}
        <tr class="hover:bg-gray-50">
            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-indigo-600">
                <a href="/queue/{{.Name}}" class="hover:text-indigo-800">{{.Name}}</a>
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 text-center">
                {{if gt .Wait 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=waiting" class="text-yellow-600 hover:text-yellow-800">{{.Wait}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Wait}}</span>
                {{end}}
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center">
                {{if gt .Active 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=active" class="text-blue-600 font-bold hover:text-blue-800">{{.Active}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Active}}</span>
                {{end}}
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center">
                {{if gt .Completed 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=completed" class="text-green-600 hover:text-green-800">{{.Completed}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Completed}}</span>
                {{end}}
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center">
                <span class="px-2 py-1 rounded text-xs {{if gt .Failed 0}}bg-red-100 text-red-800 font-bold{{else}}bg-gray-100 text-gray-400{{end}}">
                    {{if gt .Failed 0}}
                        <a href="/queue/jobs?queue={{.Name}}&state=failed" class="text-red-800 hover:text-red-900">{{.Failed}}</a>
                    {{else}}
                        {{.Failed}}
                    {{end}}
                </span>
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center text-purple-600">
                {{if gt .Delayed 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=delayed" class="hover:text-purple-800">{{.Delayed}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Delayed}}</span>
                {{end}}
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-sm text-center">
                <a href="/queue/{{.Name}}" class="text-indigo-600 hover:text-indigo-900">View →</a>
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
			log.Printf("❌ DiscoverQueues error: %v", err)
			http.Error(w, fmt.Sprintf("DiscoverQueues error: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Found %d queues: %v", len(queues), queues)

		queueStats, err := exp.GetQueueStats(r.Context(), queues)
		if err != nil {
			log.Printf("❌ GetQueueStats error: %v", err)
			http.Error(w, fmt.Sprintf("GetQueueStats error: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Got stats for %d queues: %+v", len(queueStats), queueStats)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err = tmpl.Execute(w, queueStats)
		if err != nil {
			log.Printf("❌ Template execution error: %v", err)
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ Template rendered successfully")
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
			_, err := fmt.Fprintf(w, "Redis unavailable: %v", err)
			if err != nil {
				return
			}
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

type pageData struct {
	Title    string
	Subtitle string
	Data     interface{}
}

const shellTmpl = `
<!DOCTYPE html>
<html>
    <head>
        <script src="https://unpkg.com/htmx.org@1.9.10"></script>
        <script src="https://cdn.tailwindcss.com"></script>
        <title>{{.Title}}</title>
    </head>
    <body class="bg-gray-50 p-10">
        <div class="max-w-6xl mx-auto bg-white shadow rounded-lg p-6">
            <div class="flex justify-between items-center mb-6">
                <div>
                    <h1 class="text-2xl font-bold text-indigo-600">🐂 Bullderdash Explorer</h1>
                    {{if .Subtitle}}<div class="text-sm text-gray-500">{{.Subtitle}}</div>{{end}}
                </div>
                <div class="flex gap-4 text-sm text-gray-600">
                    <a href="/" class="hover:text-indigo-600">Home</a>
                    <a href="/metrics" target="_blank" class="hover:text-indigo-600">📊 Metrics</a>
                    <a href="/health" target="_blank" class="hover:text-indigo-600">💚 Health</a>
                </div>
            </div>

            {{template "content" .}}
        </div>
    </body>
</html>
`

const homeContentTmpl = `
<div id="queue-list" hx-get="/queues" hx-trigger="load, every 5s">
    Loading queues...
</div>
`

func renderShell(w http.ResponseWriter, title, subtitle, contentTmpl string, data interface{}) error {
	tmpl, err := template.New("shell").Parse(shellTmpl)
	if err != nil {
		return err
	}

	wrappedContent := "{{define \"content\"}}" + contentTmpl + "{{end}}"
	if _, err := tmpl.Parse(wrappedContent); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "shell", pageData{
		Title:    title,
		Subtitle: subtitle,
		Data:     data,
	})
}

// HomeHandler renders the main dashboard shell
func HomeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := renderShell(w, "Bull-der-dash", "", homeContentTmpl, nil)
		if err != nil {
			log.Printf("❌ renderShell error (home): %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// QueueDetailHandler shows detailed view of a single queue with all job states
func QueueDetailHandler(exp *explorer.Explorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract queue name from path: /queue/{name}
		queueName := strings.TrimPrefix(r.URL.Path, "/queue/")
		if queueName == "" {
			http.Error(w, "queue name required", http.StatusBadRequest)
			return
		}

		// Get stats for this queue only
		stats, err := exp.GetQueueStats(r.Context(), []string{queueName})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(stats) == 0 {
			http.Error(w, "queue not found", http.StatusNotFound)
			return
		}

		stat := stats[0]

		// Get jobs in each state
		waiting, _ := exp.GetJobsByState(r.Context(), queueName, "waiting", 50)
		active, _ := exp.GetJobsByState(r.Context(), queueName, "active", 50)
		completed, _ := exp.GetJobsByState(r.Context(), queueName, "completed", 50)
		failed, _ := exp.GetJobsByState(r.Context(), queueName, "failed", 50)
		delayed, _ := exp.GetJobsByState(r.Context(), queueName, "delayed", 50)

		data := struct {
			Stat      explorer.QueueStats
			Waiting   []explorer.JobSummary
			Active    []explorer.JobSummary
			Completed []explorer.JobSummary
			Failed    []explorer.JobSummary
			Delayed   []explorer.JobSummary
		}{
			Stat:      stat,
			Waiting:   waiting,
			Active:    active,
			Completed: completed,
			Failed:    failed,
			Delayed:   delayed,
		}

		if r.Header.Get("HX-Request") != "" {
			tmpl := template.Must(template.New("queue-detail").Parse(queueDetailTmpl))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := tmpl.Execute(w, pageData{Data: data}); err != nil {
				log.Printf("❌ queue detail fragment render error (queue=%s): %v", queueName, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}

		err = renderShell(w, "Bull-der-dash - "+queueName, "Queue: "+queueName, queueDetailTmpl, data)
		if err != nil {
			log.Printf("❌ renderShell error (queue=%s): %v", queueName, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

const queueDetailTmpl = `
<div id="queue-detail" hx-get="/queue/{{.Data.Stat.Name}}" hx-trigger="every 5s" hx-swap="outerHTML">
<table class="min-w-full divide-y divide-gray-200 mb-8">
    <thead class="bg-gray-50">
        <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">State</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider text-center">Count</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Preview</th>
        </tr>
    </thead>
    <tbody class="bg-white divide-y divide-gray-200">
        <tr>
            <td class="px-6 py-4 text-sm text-yellow-700 font-semibold">Waiting</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Wait}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Waiting}}{{(index .Data.Waiting 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-blue-700 font-semibold">Active</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Active}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Active}}{{(index .Data.Active 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-green-700 font-semibold">Completed</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Completed}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Completed}}{{(index .Data.Completed 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-red-700 font-semibold">Failed</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Failed}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Failed}}{{(index .Data.Failed 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-purple-700 font-semibold">Delayed</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Delayed}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Delayed}}{{(index .Data.Delayed 0).ID}}{{else}}—{{end}}</td>
        </tr>
    </tbody>
</table>

<div class="space-y-8">
    {{if .Data.Waiting}}
    <div>
        <h2 class="text-lg font-semibold text-yellow-700 mb-3">Waiting</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Waiting}}
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                    <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm"><a href="/job/detail?queue={{.Queue}}&id={{.ID}}" class="text-indigo-600 hover:text-indigo-900" target="_blank">View →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Data.Active}}
    <div>
        <h2 class="text-lg font-semibold text-blue-700 mb-3">Active</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Attempts</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Active}}
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                    <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm text-gray-600">{{.AttemptsMade}}</td>
                    <td class="px-6 py-4 text-sm"><a href="/job/detail?queue={{.Queue}}&id={{.ID}}" class="text-indigo-600 hover:text-indigo-900" target="_blank">View →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Data.Delayed}}
    <div>
        <h2 class="text-lg font-semibold text-purple-700 mb-3">Delayed</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Delayed}}
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                    <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm"><a href="/job/detail?queue={{.Queue}}&id={{.ID}}" class="text-indigo-600 hover:text-indigo-900" target="_blank">View →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Data.Completed}}
    <div>
        <h2 class="text-lg font-semibold text-green-700 mb-3">Completed</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Completed}}
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                    <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm"><a href="/job/detail?queue={{.Queue}}&id={{.ID}}" class="text-indigo-600 hover:text-indigo-900" target="_blank">View →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Data.Failed}}
    <div>
        <h2 class="text-lg font-semibold text-red-700 mb-3">Failed</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Attempts</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Failed}}
                <tr class="hover:bg-gray-50">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">{{.ID}}</td>
                    <td class="px-6 py-4 text-sm text-gray-900">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm text-gray-600">{{.AttemptsMade}}</td>
                    <td class="px-6 py-4 text-sm"><a href="/job/detail?queue={{.Queue}}&id={{.ID}}" class="text-indigo-600 hover:text-indigo-900" target="_blank">View →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}
</div>
`
