package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kofno/bullderdash/internal/explorer"
)

// The template for our queue list
const queueListTmpl = `
<div class="grid cols-auto">
    {{range .}}
    <div class="card">
        <div class="qcard-head">
            <a href="/queue/{{.Name}}" class="qname">{{.Name}}</a>
            <span class="qtotal">Total<strong>{{.Total}}</strong></span>
        </div>
        <div class="qgrid">
            <div class="qcell g-warn">
                <span class="k">Waiting</span>
                {{if gt .Wait 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=waiting">{{.Wait}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-flow">
                <span class="k">Active</span>
                {{if gt .Active 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=active">{{.Active}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-muted">
                <span class="k">Paused</span>
                {{if gt .Paused 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=paused">{{.Paused}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-flow">
                <span class="k">Prioritized</span>
                {{if gt .Prioritized 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=prioritized">{{.Prioritized}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-warn">
                <span class="k">Waiting-Children</span>
                {{if gt .WaitingChildren 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=waiting-children">{{.WaitingChildren}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-ok">
                <span class="k">Completed</span>
                {{if gt .Completed 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=completed">{{.Completed}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-danger">
                <span class="k">Failed</span>
                {{if gt .Failed 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=failed">{{.Failed}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-warn">
                <span class="k">Delayed</span>
                {{if gt .Delayed 0}}<a class="v" href="/queue/jobs?queue={{.Name}}&state=delayed">{{.Delayed}}</a>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-danger">
                <span class="k">Stalled</span>
                {{if gt .Stalled 0}}<span class="v">{{.Stalled}}</span>{{else}}<span class="v zero">0</span>{{end}}
            </div>
            <div class="qcell g-muted">
                <span class="k">Orphaned</span>
                {{if .OrphanedKnown}}{{if gt .Orphaned 0}}<span class="v">{{.Orphaned}}</span>{{else}}<span class="v zero">0</span>{{end}}{{else}}<span class="v zero" title="Orphaned is available in diagnostic views">diag</span>{{end}}
            </div>
        </div>
        <div class="qcard-foot">
            <a class="link" href="/queue/{{.Name}}">View →</a>
        </div>
    </div>
    {{end}}
</div>
`

func DashboardHandler(exp *explorer.Explorer, prefix string, cache *DashboardCache) http.HandlerFunc {
	tmpl := template.Must(template.New("queues").Parse(queueListTmpl))

	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := cache.Get()
		if len(snapshot.Stats) == 0 {
			if err := RefreshDashboardCache(r.Context(), exp, prefix, cache); err != nil {
				log.Printf("❌ dashboard snapshot refresh error: %v", err)
				http.Error(w, fmt.Sprintf("Dashboard snapshot unavailable: %v", err), http.StatusServiceUnavailable)
				return
			}
			snapshot = cache.Get()
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := tmpl.Execute(w, snapshot.Stats)
		if err != nil {
			log.Printf("❌ Template execution error: %v", err)
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// JobListHandler shows jobs in a specific state for a queue
func JobListHandler(exp *explorer.Explorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const (
			statePageSize     = 100
			allStatesPageSize = 50
		)

		queueName := r.URL.Query().Get("queue")
		state := r.URL.Query().Get("state")
		if queueName == "" || state == "" {
			http.Error(w, "queue and state parameters required", http.StatusBadRequest)
			return
		}

		displayState := state
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		offset := 0
		limit := statePageSize
		searchedJobs := 0
		hasNextPage := false
		windowLabel := ""
		var jobs []explorer.JobSummary
		var err error

		switch {
		case state == "all":
			displayState = "all"
			limit = allStatesPageSize
			offset = (page - 1) * allStatesPageSize
			jobs, err = exp.GetJobsAcrossStatesPage(r.Context(), queueName, offset, limit)
			searchedJobs = len(jobs)
			windowLabel = fmt.Sprintf("Showing jobs %d-%d from each state", offset+1, offset+limit)
			hasNextPage = len(jobs) == limit
		default:
			offset = (page - 1) * statePageSize
			jobs, err = exp.GetJobsByStatePage(r.Context(), queueName, state, offset, limit)
			searchedJobs = len(jobs)
			windowLabel = fmt.Sprintf("Showing jobs %d-%d in %s", offset+1, offset+limit, state)
			hasNextPage = len(jobs) == limit
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			Queue        string
			State        string
			Jobs         []explorer.JobSummary
			Page         int
			HasPrevPage  bool
			HasNextPage  bool
			WindowLabel  string
			SearchedJobs int
		}{
			Queue:        queueName,
			State:        displayState,
			Jobs:         jobs,
			Page:         page,
			HasPrevPage:  page > 1,
			HasNextPage:  hasNextPage,
			WindowLabel:  windowLabel,
			SearchedJobs: searchedJobs,
		}

		if r.Header.Get("HX-Request") != "" {
			tmpl := template.Must(template.New("jobs").Funcs(template.FuncMap{
				"add": func(a, b int) int { return a + b },
				"sub": func(a, b int) int { return a - b },
			}).Parse(jobListTmpl))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := tmpl.Execute(w, pageData{Data: data}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}

		err = renderShell(w, "Bull-der-dash - "+queueName, "Queue: "+queueName+" / "+state, "overview", jobListTmpl, data)
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
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		err := exp.Ping(ctx)
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
    <div class="page-head">
        <div class="row" style="justify-content:space-between;">
            <div>
                <h1>{{.Data.Queue}}</h1>
                <p><span class="state {{.Data.State}}">{{.Data.State}}</span> · {{len .Data.Jobs}} job(s) on this page</p>
            </div>
            <div class="row">
                <a class="btn secondary" href="/queue/{{.Data.Queue}}">← Back to Queue</a>
                <a class="btn secondary" href="/">All Queues</a>
                {{if ne .Data.State "all"}}
                <a class="btn secondary" href="/queue/jobs?queue={{.Data.Queue}}&state=all">All States</a>
                {{end}}
            </div>
        </div>
    </div>

    {{if .Data.WindowLabel}}
    <div class="notice info row" style="justify-content:space-between;">
        <span>{{.Data.WindowLabel}}</span>
        <span class="muted">{{.Data.SearchedJobs}} jobs loaded for this page</span>
    </div>
    {{end}}

    {{if .Data.Jobs}}
    <div class="table-wrap">
        <table>
            <thead>
                <tr>
                    <th>Job ID</th>
                    <th>Name</th>
                    <th>Created</th>
                    <th>Attempts</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                {{range .Data.Jobs}}
                <tr>
                    <td class="mono">{{.ID}}</td>
                    <td>{{.Name}}</td>
                    <td class="muted">{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
                    <td>{{.AttemptsMade}}</td>
                    <td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View Details →</a></td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{else}}
    <div class="empty">No jobs in {{.Data.State}} state</div>
    {{end}}

    <div class="row" style="justify-content:space-between;">
        <div class="muted">Page {{.Data.Page}}</div>
        <div class="row">
            {{if .Data.HasPrevPage}}
            <a class="btn secondary" href="/queue/jobs?queue={{.Data.Queue}}&state={{.Data.State}}&page={{sub .Data.Page 1}}">Previous</a>
            {{end}}
            {{if .Data.HasNextPage}}
            <a class="btn secondary" href="/queue/jobs?queue={{.Data.Queue}}&state={{.Data.State}}&page={{add .Data.Page 1}}">Next</a>
            {{end}}
        </div>
    </div>
</div>
`

type pageData struct {
	Title    string
	Subtitle string
	Data     interface{}
	// ConsoleEnabled mirrors whether the SQLite-backed search console route is
	// mounted (store enabled). It gates the nav link so disabled deployments
	// don't advertise a 404.
	ConsoleEnabled bool
	// AssetVer is the per-process build id appended to static asset URLs so a
	// redeploy busts browser caches of the immutable /assets/ files.
	AssetVer string
	// NavActive marks the current top-nav section ("overview", "console", …) so
	// the shell can highlight it. Empty means no item is highlighted.
	NavActive string
}

// ConsoleEnabled is set true by main at startup when the job-history store (and
// therefore the /console route) is active. It controls the layout nav link.
var ConsoleEnabled bool

const shellTmpl = `
<!DOCTYPE html>
<html lang="en">
    <head>
        <meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1">
        <title>{{.Title}}</title>
        <link rel="stylesheet" href="/assets/app.css?v={{.AssetVer}}">
    </head>
    <body>
        <header class="app-header">
            <div class="brand">
                <a class="title" href="/">Bull-der-dash</a>
                <span class="subtitle">{{if .Subtitle}}{{.Subtitle}}{{else}}BullMQ / Valkey observability{{end}}</span>
            </div>
            <nav class="app-nav">
                <a href="/"{{if eq .NavActive "overview"}} class="active"{{end}}>Overview</a>
                {{if .ConsoleEnabled}}<a href="/console"{{if eq .NavActive "console"}} class="active"{{end}}>Console</a>{{end}}
                <a href="/metrics" target="_blank" rel="noopener">Metrics</a>
                <a href="/health" target="_blank" rel="noopener">Health</a>
            </nav>
        </header>
        <main class="container">
            {{template "content" .}}
        </main>
        <script src="/assets/app.js?v={{.AssetVer}}" defer></script>
    </body>
</html>
`

const homeContentTmpl = `
<div class="page-head">
    <h1>Overview</h1>
    <p>Live queue depths and throughput, refreshed automatically.</p>
</div>
<div id="queue-list" data-poll-url="/queues" data-poll-interval="5000">
    <div class="empty">Loading queues…</div>
</div>
`

func renderShell(w http.ResponseWriter, title, subtitle, navActive, contentTmpl string, data interface{}) error {
	tmpl, err := template.New("shell").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}).Parse(shellTmpl)
	if err != nil {
		return err
	}

	wrappedContent := "{{define \"content\"}}" + contentTmpl + "{{end}}"
	if _, err := tmpl.Parse(wrappedContent); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "shell", pageData{
		Title:          title,
		Subtitle:       subtitle,
		Data:           data,
		ConsoleEnabled: ConsoleEnabled,
		AssetVer:       AssetBuildID(),
		NavActive:      navActive,
	})
}

func parsePositiveInt(raw string, fallback int) int {
	if fallback < 1 {
		fallback = 1
	}
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

// HomeHandler renders the main dashboard shell
func HomeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := renderShell(w, "Bull-der-dash", "", "overview", homeContentTmpl, nil)
		if err != nil {
			log.Printf("❌ renderShell error (home): %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// queueDetailPageData describes the queue detail view payload.
type queueDetailPageData struct {
	Stat            explorer.QueueStats
	SummaryHTML     template.HTML
	Waiting         []explorer.JobSummary
	Active          []explorer.JobSummary
	Paused          []explorer.JobSummary
	Prioritized     []explorer.JobSummary
	WaitingChildren []explorer.JobSummary
	Completed       []explorer.JobSummary
	Failed          []explorer.JobSummary
	Delayed         []explorer.JobSummary
}

type queueSummaryViewData struct {
	Stat explorer.QueueStats
}

// QueueSummaryHandler renders the fast, polled summary for a queue.
func QueueSummaryHandler(exp *explorer.Explorer, prefix string) http.HandlerFunc {
	tmpl := template.Must(template.New("queue-summary").Parse(queueSummaryTmpl))

	return func(w http.ResponseWriter, r *http.Request) {
		queueName := strings.TrimSpace(r.URL.Query().Get("queue"))
		if queueName == "" {
			http.Error(w, "queue parameter required", http.StatusBadRequest)
			return
		}

		stat, err := loadFastQueueStat(r.Context(), exp, prefix, queueName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, pageData{Data: queueSummaryViewData{Stat: stat}}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// QueueDetailHandler shows detailed view of a single queue with all job states
func QueueDetailHandler(exp *explorer.Explorer, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract queue name from path: /queue/{name}
		queueName := strings.TrimPrefix(r.URL.Path, "/queue/")
		if queueName == "" {
			http.Error(w, "queue name required", http.StatusBadRequest)
			return
		}

		stat, err := loadFastQueueStat(r.Context(), exp, prefix, queueName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get jobs in each state
		waiting, _ := exp.GetJobsByState(r.Context(), queueName, "waiting", 50)
		active, _ := exp.GetJobsByState(r.Context(), queueName, "active", 50)
		paused, _ := exp.GetJobsByState(r.Context(), queueName, "paused", 50)
		prioritized, _ := exp.GetJobsByState(r.Context(), queueName, "prioritized", 50)
		waitingChildren, _ := exp.GetJobsByState(r.Context(), queueName, "waiting-children", 50)
		completed, _ := exp.GetJobsByState(r.Context(), queueName, "completed", 50)
		failed, _ := exp.GetJobsByState(r.Context(), queueName, "failed", 50)
		delayed, _ := exp.GetJobsByState(r.Context(), queueName, "delayed", 50)

		summaryHTML, err := renderQueueSummaryHTML(stat)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := queueDetailPageData{
			Stat:            stat,
			SummaryHTML:     summaryHTML,
			Waiting:         waiting,
			Active:          active,
			Paused:          paused,
			Prioritized:     prioritized,
			WaitingChildren: waitingChildren,
			Completed:       completed,
			Failed:          failed,
			Delayed:         delayed,
		}

		err = renderShell(w, "Bull-der-dash - "+queueName, "Queue: "+queueName, "overview", queueDetailTmpl, data)
		if err != nil {
			log.Printf("❌ renderShell error (queue=%s): %v", queueName, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func loadFastQueueStat(ctx context.Context, exp *explorer.Explorer, prefix, queueName string) (explorer.QueueStats, error) {
	stats, err := exp.GetQueueStatsFast(ctx, prefix, []string{queueName})
	if err != nil {
		return explorer.QueueStats{}, err
	}
	if len(stats) == 0 {
		return explorer.QueueStats{}, fmt.Errorf("queue not found")
	}
	return stats[0], nil
}

func renderQueueSummaryHTML(stat explorer.QueueStats) (template.HTML, error) {
	tmpl, err := template.New("queue-summary").Parse(queueSummaryTmpl)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, pageData{Data: queueSummaryViewData{Stat: stat}}); err != nil {
		return "", err
	}

	return template.HTML(b.String()), nil
}

const queueSummaryTmpl = `
<div class="grid cols-auto" style="margin-bottom:20px;">
    <div class="card">
        <div class="stat">
            <span class="label">Queue</span>
            <span class="value" style="font-size:18px;">{{.Data.Stat.Name}}</span>
        </div>
        <div class="stat" style="margin-top:10px;">
            <span class="label">Total jobs</span>
            <span class="value">{{.Data.Stat.Total}}</span>
        </div>
        <div style="margin-top:12px;">
            <a class="btn" href="/console" target="_blank" rel="noopener">Open Console →</a>
        </div>
    </div>
    <div class="card">
        <h3>Flow</h3>
        <div class="qgrid">
            <div class="qcell g-warn"><span class="k">Waiting</span><span class="v">{{.Data.Stat.Wait}}</span></div>
            <div class="qcell g-flow"><span class="k">Active</span><span class="v">{{.Data.Stat.Active}}</span></div>
            <div class="qcell g-warn"><span class="k">Delayed</span><span class="v">{{.Data.Stat.Delayed}}</span></div>
            <div class="qcell g-ok"><span class="k">Completed</span><span class="v">{{.Data.Stat.Completed}}</span></div>
        </div>
    </div>
    <div class="card">
        <h3>Exceptions</h3>
        <div class="qgrid">
            <div class="qcell g-danger"><span class="k">Failed</span><span class="v">{{.Data.Stat.Failed}}</span></div>
            <div class="qcell g-danger"><span class="k">Stalled</span><span class="v">{{.Data.Stat.Stalled}}</span></div>
            <div class="qcell g-muted"><span class="k">Orphaned</span>{{if .Data.Stat.OrphanedKnown}}<span class="v">{{.Data.Stat.Orphaned}}</span>{{else}}<span class="v zero" title="Orphaned is available in diagnostics">diag</span>{{end}}</div>
            <div class="qcell g-muted"><span class="k">Paused</span><span class="v">{{.Data.Stat.Paused}}</span></div>
        </div>
    </div>
</div>
`

const queueDetailTmpl = `
<div id="queue-detail" class="space-y-6">
<div data-poll-url="/queue/summary?queue={{.Data.Stat.Name}}" data-poll-interval="5000">
{{.Data.SummaryHTML}}
</div>

<div class="table-wrap" style="margin-bottom:20px;">
    <table>
        <thead>
            <tr><th>State</th><th>Count</th><th>Preview</th></tr>
        </thead>
        <tbody>
            <tr><td><span class="state Waiting">Waiting</span></td><td>{{.Data.Stat.Wait}}</td><td class="mono muted">{{if .Data.Waiting}}{{(index .Data.Waiting 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Active">Active</span></td><td>{{.Data.Stat.Active}}</td><td class="mono muted">{{if .Data.Active}}{{(index .Data.Active 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Paused">Paused</span></td><td>{{.Data.Stat.Paused}}</td><td class="mono muted">{{if .Data.Paused}}{{(index .Data.Paused 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Prioritized">Prioritized</span></td><td>{{.Data.Stat.Prioritized}}</td><td class="mono muted">{{if .Data.Prioritized}}{{(index .Data.Prioritized 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state WaitingChildren">Waiting-Children</span></td><td>{{.Data.Stat.WaitingChildren}}</td><td class="mono muted">{{if .Data.WaitingChildren}}{{(index .Data.WaitingChildren 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Completed">Completed</span></td><td>{{.Data.Stat.Completed}}</td><td class="mono muted">{{if .Data.Completed}}{{(index .Data.Completed 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Failed">Failed</span></td><td>{{.Data.Stat.Failed}}</td><td class="mono muted">{{if .Data.Failed}}{{(index .Data.Failed 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Delayed">Delayed</span></td><td>{{.Data.Stat.Delayed}}</td><td class="mono muted">{{if .Data.Delayed}}{{(index .Data.Delayed 0).ID}}{{else}}—{{end}}</td></tr>
            <tr><td><span class="state Stalled">Stalled</span></td><td>{{.Data.Stat.Stalled}}</td><td class="muted">—</td></tr>
            <tr><td><span class="state Orphaned">Orphaned</span></td><td>{{if .Data.Stat.OrphanedKnown}}{{.Data.Stat.Orphaned}}{{else}}<span class="muted">diag</span>{{end}}</td><td class="muted">—</td></tr>
            <tr><td><span class="state Total">Total</span></td><td>{{.Data.Stat.Total}}</td><td class="muted">—</td></tr>
        </tbody>
    </table>
</div>

<div class="space-y-8">
    {{if .Data.Waiting}}
    <div>
        <div class="section-head">Waiting</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Waiting}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Active}}
    <div>
        <div class="section-head">Active</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Attempts</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Active}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td>{{.AttemptsMade}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Paused}}
    <div>
        <div class="section-head">Paused</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Paused}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Prioritized}}
    <div>
        <div class="section-head">Prioritized</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Prioritized}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.WaitingChildren}}
    <div>
        <div class="section-head">Waiting-Children</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.WaitingChildren}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Delayed}}
    <div>
        <div class="section-head">Delayed</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Delayed}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Completed}}
    <div>
        <div class="section-head">Completed</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Completed}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}

    {{if .Data.Failed}}
    <div>
        <div class="section-head">Failed</div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Job ID</th><th>Name</th><th>Attempts</th><th>Actions</th></tr></thead>
                <tbody>
                    {{range .Data.Failed}}
                    <tr><td class="mono">{{.ID}}</td><td>{{.Name}}</td><td>{{.AttemptsMade}}</td><td><a class="link" href="/job/detail?queue={{.Queue}}&id={{.ID}}" target="_blank" rel="noopener">View →</a></td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    {{end}}
</div>
</div>
`
