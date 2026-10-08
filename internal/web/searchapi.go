package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kofno/bullderdash/internal/store"
)

// StoreReader is the read surface of the SQLite job-history store consumed by
// the JSON console API. Kept as an interface so the handlers can be tested
// without a real database.
type StoreReader interface {
	Search(ctx context.Context, p store.SearchParams) ([]store.SearchRow, error)
	Get(ctx context.Context, id string) (*store.JobDetail, error)
}

// SearchAPI serves the AnvilMQ-console-compatible JSON endpoints
// (GET /v1/search and GET /v1/jobs/{id}) over the job-history store. A bounded
// semaphore caps concurrent reads and returns 503 when saturated so a burst of
// expensive queries cannot exhaust the single SQLite writer/reader budget.
type SearchAPI struct {
	reader StoreReader
	sem    chan struct{}
}

// NewSearchAPI builds a SearchAPI. maxConcurrent <= 0 defaults to 16.
func NewSearchAPI(reader StoreReader, maxConcurrent int) *SearchAPI {
	if maxConcurrent <= 0 {
		maxConcurrent = 16
	}
	return &SearchAPI{
		reader: reader,
		sem:    make(chan struct{}, maxConcurrent),
	}
}

// acquire takes a slot without blocking. It returns false when the reader is
// saturated, letting the caller reply 503 per the console contract.
func (a *SearchAPI) acquire() bool {
	select {
	case a.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *SearchAPI) release() { <-a.sem }

// SearchHandler implements GET /v1/search. At least one of q, name, state or
// trace_id must be supplied (since_ms/limit alone are not predicates), else 400.
func (a *SearchAPI) SearchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
		if q == "" && name == "" && state == "" && traceID == "" {
			writeJSONError(w, http.StatusBadRequest, "at least one of q, name, state or trace_id is required")
			return
		}

		params := store.SearchParams{
			Query:   q,
			Name:    name,
			State:   state,
			TraceID: traceID,
			SinceMs: parseInt64(r.URL.Query().Get("since_ms")),
			Limit:   parseIntDefault(r.URL.Query().Get("limit"), 0),
		}

		if !a.acquire() {
			writeJSONError(w, http.StatusServiceUnavailable, "search reader busy")
			return
		}
		defer a.release()

		rows, err := a.reader.Search(r.Context(), params)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "search failed")
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// JobDetailHandler implements GET /v1/jobs/{id}.
func (a *SearchAPI) JobDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"))
		id = strings.Trim(id, "/")
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "job id is required")
			return
		}

		if !a.acquire() {
			writeJSONError(w, http.StatusServiceUnavailable, "search reader busy")
			return
		}
		defer a.release()

		detail, err := a.reader.Get(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "job not found")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseInt64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseIntDefault(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}
