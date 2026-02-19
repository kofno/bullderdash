package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kofno/bullderdash/internal/explorer"
)

// The template for our queue list
const queueListTmpl = `
<div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
    {{range .}}
    <div class="rounded-xl border border-gray-200 bg-white shadow-sm hover:shadow-md transition-shadow">
        <div class="px-4 py-3 border-b border-gray-100 flex items-center justify-between">
            <a href="/queue/{{.Name}}" class="text-lg font-semibold text-indigo-700 hover:text-indigo-900">{{.Name}}</a>
            <span class="text-xs uppercase tracking-wide text-gray-400">Total</span>
            <span class="text-sm font-bold text-gray-900">{{.Total}}</span>
        </div>
        <div class="px-4 py-3 grid grid-cols-2 gap-2 text-sm">
            <div class="flex items-center justify-between rounded-md bg-yellow-50 px-2 py-1">
                <span class="text-yellow-800">Waiting</span>
                {{if gt .Wait 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=waiting" class="font-semibold text-yellow-900 hover:text-yellow-700">{{.Wait}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Wait}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-blue-50 px-2 py-1">
                <span class="text-blue-800">Active</span>
                {{if gt .Active 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=active" class="font-semibold text-blue-900 hover:text-blue-700">{{.Active}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Active}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-slate-50 px-2 py-1">
                <span class="text-slate-700">Paused</span>
                {{if gt .Paused 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=paused" class="font-semibold text-slate-800 hover:text-slate-600">{{.Paused}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Paused}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-fuchsia-50 px-2 py-1">
                <span class="text-fuchsia-800">Prioritized</span>
                {{if gt .Prioritized 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=prioritized" class="font-semibold text-fuchsia-900 hover:text-fuchsia-700">{{.Prioritized}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Prioritized}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-amber-50 px-2 py-1">
                <span class="text-amber-800">Waiting-Children</span>
                {{if gt .WaitingChildren 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=waiting-children" class="font-semibold text-amber-900 hover:text-amber-700">{{.WaitingChildren}}</a>
                {{else}}
                    <span class="text-gray-400">{{.WaitingChildren}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-green-50 px-2 py-1">
                <span class="text-green-800">Completed</span>
                {{if gt .Completed 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=completed" class="font-semibold text-green-900 hover:text-green-700">{{.Completed}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Completed}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-red-50 px-2 py-1">
                <span class="text-red-800">Failed</span>
                {{if gt .Failed 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=failed" class="font-semibold text-red-900 hover:text-red-700">{{.Failed}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Failed}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-purple-50 px-2 py-1">
                <span class="text-purple-800">Delayed</span>
                {{if gt .Delayed 0}}
                    <a href="/queue/jobs?queue={{.Name}}&state=delayed" class="font-semibold text-purple-900 hover:text-purple-700">{{.Delayed}}</a>
                {{else}}
                    <span class="text-gray-400">{{.Delayed}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-orange-50 px-2 py-1">
                <span class="text-orange-800">Stalled</span>
                {{if gt .Stalled 0}}
                    <span class="font-semibold text-orange-900">{{.Stalled}}</span>
                {{else}}
                    <span class="text-gray-400">{{.Stalled}}</span>
                {{end}}
            </div>
            <div class="flex items-center justify-between rounded-md bg-gray-100 px-2 py-1">
                <span class="text-gray-700">Orphaned</span>
                {{if gt .Orphaned 0}}
                    <span class="font-semibold text-gray-900">{{.Orphaned}}</span>
                {{else}}
                    <span class="text-gray-400">{{.Orphaned}}</span>
                {{end}}
            </div>
        </div>
        <div class="px-4 py-3 border-t border-gray-100 text-right">
            <a href="/queue/{{.Name}}" class="text-sm font-medium text-indigo-600 hover:text-indigo-900">View →</a>
        </div>
    </div>
    {{end}}
</div>
`

func DashboardHandler(exp *explorer.Explorer, prefix string) http.HandlerFunc {
	tmpl := template.Must(template.New("queues").Parse(queueListTmpl))

	return func(w http.ResponseWriter, r *http.Request) {
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
	return func(w http.ResponseWriter, r *http.Request) {
		queueName := r.URL.Query().Get("queue")
		state := r.URL.Query().Get("state")
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if queueName == "" || state == "" {
			http.Error(w, "queue and state parameters required", http.StatusBadRequest)
			return
		}

		displayState := state
		limit := 100
		var jobs []explorer.JobSummary
		var err error
		if state == "all" || query != "" {
			displayState = "all"
			limit = 200
			jobs, err = exp.GetJobsAcrossStates(r.Context(), queueName, limit)
		} else {
			jobs, err = exp.GetJobsByState(r.Context(), queueName, state, limit)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if query != "" {
			queryLower := strings.ToLower(query)
			filtered := jobs[:0]
			for _, job := range jobs {
				if strings.Contains(strings.ToLower(job.ID), queryLower) ||
					strings.Contains(strings.ToLower(job.Name), queryLower) ||
					strings.Contains(strings.ToLower(job.Data), queryLower) ||
					strings.Contains(strings.ToLower(job.Opts), queryLower) ||
					strings.Contains(strings.ToLower(job.FailedReason), queryLower) {
					filtered = append(filtered, job)
				}
			}
			jobs = filtered
		}

		data := struct {
			Queue string
			State string
			Query string
			Jobs  []explorer.JobSummary
		}{
			Queue: queueName,
			State: displayState,
			Query: query,
			Jobs:  jobs,
		}

		if r.Header.Get("HX-Request") != "" {
			tmpl := template.Must(template.New("jobs").Parse(jobListTmpl))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := tmpl.Execute(w, pageData{Data: data}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}

		err = renderShell(w, "Bull-der-dash - "+queueName, "Queue: "+queueName+" / "+state, jobListTmpl, data)
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
<div class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
            <div class="text-sm uppercase tracking-wide text-gray-400">Queue</div>
            <div class="text-xl font-semibold text-indigo-700">{{.Data.Queue}}</div>
        </div>
        <div class="flex items-center gap-3">
            <span class="text-xs uppercase tracking-wide text-gray-400">State</span>
            <span class="px-2 py-1 rounded-full text-xs bg-gray-100 text-gray-700">{{.Data.State}}</span>
            <span class="text-sm text-gray-500">({{len .Data.Jobs}})</span>
        </div>
        <div class="flex items-center gap-4 text-sm">
            <a href="/queue/{{.Data.Queue}}" class="font-medium text-indigo-600 hover:text-indigo-800">← Back to Queue</a>
            <a href="/" class="font-medium text-gray-500 hover:text-gray-700">All Queues</a>
            {{if ne .Data.State "all"}}
            <a href="/queue/jobs?queue={{.Data.Queue}}&state=all" class="font-medium text-gray-500 hover:text-gray-700">All States View</a>
            {{end}}
        </div>
    </div>

    <form class="flex flex-wrap items-end gap-3" method="get" action="/queue/jobs">
        <input type="hidden" name="queue" value="{{.Data.Queue}}">
        <input type="hidden" name="state" value="{{.Data.State}}">
        <label class="flex flex-col text-xs uppercase tracking-wide text-gray-400">
            Search Jobs
            <span class="mt-1 text-[10px] normal-case text-gray-400">Searches all states</span>
            <input
                type="text"
                name="q"
                value="{{.Data.Query}}"
                placeholder="Job ID or name (all states)"
                class="mt-1 w-64 rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-800 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
        </label>
        <button
            type="submit"
            class="h-9 rounded-md bg-indigo-600 px-4 text-sm font-medium text-white hover:bg-indigo-700"
        >
            Search
        </button>
        {{if .Data.Query}}
        <a
            href="/queue/jobs?queue={{.Data.Queue}}&state={{.Data.State}}"
            class="h-9 rounded-md border border-gray-300 px-3 text-sm font-medium text-gray-600 hover:text-gray-900 flex items-center"
        >
            Clear
        </a>
        {{end}}
    </form>

    {{if .Data.Jobs}}
    <div class="overflow-x-auto rounded-lg border border-gray-200">
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
                {{range .Data.Jobs}}
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
    </div>
    {{else}}
    <div class="text-center py-12 text-gray-500 border border-dashed border-gray-200 rounded-lg">
        {{if .Data.Query}}
            No jobs matching "{{.Data.Query}}"
        {{else}}
            No jobs in {{.Data.State}} state
        {{end}}
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
                <a href="/search" class="font-medium text-indigo-600 hover:text-indigo-800">Search Jobs</a>
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

const searchPageTmpl = `
<div class="space-y-6">
    <div>
        <div class="text-sm uppercase tracking-wide text-gray-400">Search Jobs</div>
        <div class="text-xl font-semibold text-indigo-700">Find jobs across states</div>
    </div>

    <form class="flex flex-wrap items-end gap-4" method="get" action="/queue/jobs">
        <label class="flex flex-col text-xs uppercase tracking-wide text-gray-400">
            Queue
            <select
                name="queue"
                class="mt-1 w-64 rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-800 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
                required
            >
                {{range .Data.Queues}}
                <option value="{{.}}" {{if eq . $.Data.SelectedQueue}}selected{{end}}>{{.}}</option>
                {{end}}
            </select>
        </label>
        <input type="hidden" name="state" value="all">
        <label class="flex flex-col text-xs uppercase tracking-wide text-gray-400">
            Query
            <input
                type="text"
                name="q"
                value="{{.Data.Query}}"
                placeholder="Job ID, name, data, opts, failedReason"
                class="mt-1 w-80 rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-800 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
        </label>
        <button
            type="submit"
            class="h-9 rounded-md bg-indigo-600 px-4 text-sm font-medium text-white hover:bg-indigo-700"
        >
            Search
        </button>
    </form>
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

// SearchPageHandler renders a global search form with queue selection
func SearchPageHandler(exp *explorer.Explorer, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queues, err := exp.DiscoverQueues(r.Context(), prefix)
		if err != nil {
			log.Printf("❌ DiscoverQueues error (search): %v", err)
			http.Error(w, fmt.Sprintf("DiscoverQueues error: %v", err), http.StatusInternalServerError)
			return
		}
		selectedQueue := strings.TrimSpace(r.URL.Query().Get("queue"))
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if selectedQueue == "" && len(queues) > 0 {
			selectedQueue = queues[0]
		}
		data := struct {
			Queues        []string
			SelectedQueue string
			Query         string
		}{
			Queues:        queues,
			SelectedQueue: selectedQueue,
			Query:         query,
		}
		err = renderShell(w, "Bull-der-dash - Search", "Search jobs across states", searchPageTmpl, data)
		if err != nil {
			log.Printf("❌ renderShell error (search): %v", err)
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
		paused, _ := exp.GetJobsByState(r.Context(), queueName, "paused", 50)
		prioritized, _ := exp.GetJobsByState(r.Context(), queueName, "prioritized", 50)
		waitingChildren, _ := exp.GetJobsByState(r.Context(), queueName, "waiting-children", 50)
		completed, _ := exp.GetJobsByState(r.Context(), queueName, "completed", 50)
		failed, _ := exp.GetJobsByState(r.Context(), queueName, "failed", 50)
		delayed, _ := exp.GetJobsByState(r.Context(), queueName, "delayed", 50)

		data := struct {
			Stat            explorer.QueueStats
			Waiting         []explorer.JobSummary
			Active          []explorer.JobSummary
			Paused          []explorer.JobSummary
			Prioritized     []explorer.JobSummary
			WaitingChildren []explorer.JobSummary
			Completed       []explorer.JobSummary
			Failed          []explorer.JobSummary
			Delayed         []explorer.JobSummary
		}{
			Stat:            stat,
			Waiting:         waiting,
			Active:          active,
			Paused:          paused,
			Prioritized:     prioritized,
			WaitingChildren: waitingChildren,
			Completed:       completed,
			Failed:          failed,
			Delayed:         delayed,
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
<div class="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-8">
    <div class="rounded-lg border border-gray-200 p-4">
        <div class="text-xs uppercase text-gray-400">Queue</div>
        <div class="text-lg font-semibold text-indigo-700">{{.Data.Stat.Name}}</div>
        <div class="mt-2 text-sm text-gray-600">Total jobs</div>
        <div class="text-2xl font-bold text-gray-900">{{.Data.Stat.Total}}</div>
        <div class="mt-4">
            <a href="/queue/jobs?queue={{.Data.Stat.Name}}&state=all" class="inline-flex items-center rounded-md bg-indigo-600 px-3 py-2 text-xs font-semibold text-white hover:bg-indigo-700">
                Search Jobs →
            </a>
        </div>
    </div>
    <div class="rounded-lg border border-gray-200 p-4">
        <div class="text-xs uppercase text-gray-400">Flow</div>
        <div class="mt-2 grid grid-cols-2 gap-2 text-sm">
            <div class="flex items-center justify-between rounded-md bg-yellow-50 px-2 py-1">
                <span class="text-yellow-800">Waiting</span>
                <span class="font-semibold text-yellow-900">{{.Data.Stat.Wait}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-blue-50 px-2 py-1">
                <span class="text-blue-800">Active</span>
                <span class="font-semibold text-blue-900">{{.Data.Stat.Active}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-purple-50 px-2 py-1">
                <span class="text-purple-800">Delayed</span>
                <span class="font-semibold text-purple-900">{{.Data.Stat.Delayed}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-green-50 px-2 py-1">
                <span class="text-green-800">Completed</span>
                <span class="font-semibold text-green-900">{{.Data.Stat.Completed}}</span>
            </div>
        </div>
    </div>
    <div class="rounded-lg border border-gray-200 p-4">
        <div class="text-xs uppercase text-gray-400">Exceptions</div>
        <div class="mt-2 grid grid-cols-2 gap-2 text-sm">
            <div class="flex items-center justify-between rounded-md bg-red-50 px-2 py-1">
                <span class="text-red-800">Failed</span>
                <span class="font-semibold text-red-900">{{.Data.Stat.Failed}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-orange-50 px-2 py-1">
                <span class="text-orange-800">Stalled</span>
                <span class="font-semibold text-orange-900">{{.Data.Stat.Stalled}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-gray-100 px-2 py-1">
                <span class="text-gray-700">Orphaned</span>
                <span class="font-semibold text-gray-900">{{.Data.Stat.Orphaned}}</span>
            </div>
            <div class="flex items-center justify-between rounded-md bg-slate-50 px-2 py-1">
                <span class="text-slate-700">Paused</span>
                <span class="font-semibold text-slate-800">{{.Data.Stat.Paused}}</span>
            </div>
        </div>
    </div>
</div>

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
            <td class="px-6 py-4 text-sm text-slate-700 font-semibold">Paused</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Paused}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Paused}}{{(index .Data.Paused 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-fuchsia-700 font-semibold">Prioritized</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.Prioritized}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.Prioritized}}{{(index .Data.Prioritized 0).ID}}{{else}}—{{end}}</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-amber-700 font-semibold">Waiting-Children</td>
            <td class="px-6 py-4 text-sm text-center">{{.Data.Stat.WaitingChildren}}</td>
            <td class="px-6 py-4 text-sm text-gray-600">{{if .Data.WaitingChildren}}{{(index .Data.WaitingChildren 0).ID}}{{else}}—{{end}}</td>
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
        <tr>
            <td class="px-6 py-4 text-sm text-orange-700 font-semibold">🔒 Stalled</td>
            <td class="px-6 py-4 text-sm text-center"><span class="px-2 py-1 rounded text-xs bg-orange-100 text-orange-800 font-bold">{{.Data.Stat.Stalled}}</span></td>
            <td class="px-6 py-4 text-sm text-gray-600">—</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-gray-700 font-semibold">👻 Orphaned</td>
            <td class="px-6 py-4 text-sm text-center"><span class="px-2 py-1 rounded text-xs bg-gray-200 text-gray-700">{{.Data.Stat.Orphaned}}</span></td>
            <td class="px-6 py-4 text-sm text-gray-600">—</td>
        </tr>
        <tr>
            <td class="px-6 py-4 text-sm text-gray-900 font-bold">📊 Total</td>
            <td class="px-6 py-4 text-sm text-center"><span class="px-2 py-1 rounded text-xs bg-gray-900 text-white font-bold">{{.Data.Stat.Total}}</span></td>
            <td class="px-6 py-4 text-sm text-gray-600">—</td>
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

    {{if .Data.Paused}}
    <div>
        <h2 class="text-lg font-semibold text-slate-700 mb-3">Paused</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Paused}}
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

    {{if .Data.Prioritized}}
    <div>
        <h2 class="text-lg font-semibold text-fuchsia-700 mb-3">Prioritized</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.Prioritized}}
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

    {{if .Data.WaitingChildren}}
    <div>
        <h2 class="text-lg font-semibold text-amber-700 mb-3">Waiting-Children</h2>
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Job ID</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {{range .Data.WaitingChildren}}
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
</div>
`
