package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kofno/bullderdash/internal/store"
)

type fakeReader struct {
	rows     []store.SearchRow
	detail   *store.JobDetail
	getErr   error
	lastP    store.SearchParams
	statRows []store.JobNameStat
	lastStat store.JobNameStatsParams
	block    chan struct{} // if set, Search blocks until closed
	entered  chan struct{} // signalled when Search is entered
}

func (f *fakeReader) Search(ctx context.Context, p store.SearchParams) ([]store.SearchRow, error) {
	f.lastP = p
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.block != nil {
		<-f.block
	}
	return f.rows, nil
}

func (f *fakeReader) Get(ctx context.Context, id string) (*store.JobDetail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}

func (f *fakeReader) TopJobNames(ctx context.Context, p store.JobNameStatsParams) ([]store.JobNameStat, error) {
	f.lastStat = p
	return f.statRows, nil
}

func TestSearchHandlerRequiresPredicate(t *testing.T) {
	api := NewSearchAPI(&fakeReader{}, 4)
	// limit alone is not a predicate -> 400.
	req := httptest.NewRequest(http.MethodGet, "/v1/search?limit=5", nil)
	rec := httptest.NewRecorder()
	api.SearchHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for no predicate, got %d", rec.Code)
	}
}

func TestSearchHandlerSinceMsIsPredicate(t *testing.T) {
	fr := &fakeReader{rows: []store.SearchRow{{ID: "j1"}}}
	api := NewSearchAPI(fr, 4)
	// A finished-within window alone is a valid standalone predicate.
	req := httptest.NewRequest(http.MethodGet, "/v1/search?since_ms=10&limit=5", nil)
	rec := httptest.NewRecorder()
	api.SearchHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for since_ms-only query, got %d", rec.Code)
	}
	if fr.lastP.SinceMs != 10 {
		t.Fatalf("since_ms not threaded: %+v", fr.lastP)
	}
}

func TestSearchHandlerParamsAndResult(t *testing.T) {
	fr := &fakeReader{rows: []store.SearchRow{{ID: "j1", Name: "N", State: "Failed"}}}
	api := NewSearchAPI(fr, 4)
	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=dev-1&name=N&state=Failed&trace_id=tr&since_ms=99&limit=7", nil)
	rec := httptest.NewRecorder()
	api.SearchHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fr.lastP.Query != "dev-1" || fr.lastP.Name != "N" || fr.lastP.State != "Failed" ||
		fr.lastP.TraceID != "tr" || fr.lastP.SinceMs != 99 || fr.lastP.Limit != 7 {
		t.Fatalf("params not threaded: %+v", fr.lastP)
	}
	var out []store.SearchRow
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(out) != 1 || out[0].ID != "j1" {
		t.Fatalf("unexpected body: %+v", out)
	}
}

func TestJobDetailHandler(t *testing.T) {
	fr := &fakeReader{detail: &store.JobDetail{SearchRow: store.SearchRow{ID: "abc"}, Data: `{"x":1}`}}
	api := NewSearchAPI(fr, 4)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/abc", nil)
	rec := httptest.NewRecorder()
	api.JobDetailHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Not found maps to 404.
	fr2 := &fakeReader{getErr: store.ErrNotFound}
	api2 := NewSearchAPI(fr2, 4)
	req2 := httptest.NewRequest(http.MethodGet, "/v1/jobs/missing", nil)
	rec2 := httptest.NewRecorder()
	api2.JobDetailHandler()(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec2.Code)
	}

	// Empty id => 400.
	api3 := NewSearchAPI(&fakeReader{}, 4)
	req3 := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	rec3 := httptest.NewRecorder()
	api3.JobDetailHandler()(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty id, got %d", rec3.Code)
	}
}

func TestSearchHandlerSaturationReturns503(t *testing.T) {
	fr := &fakeReader{block: make(chan struct{}), entered: make(chan struct{}, 1)}
	api := NewSearchAPI(fr, 1)

	// Occupy the single slot with a blocked request.
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/v1/search?q=x", nil)
		api.SearchHandler()(httptest.NewRecorder(), req)
	}()
	<-fr.entered // ensure the slot is held

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=y", nil)
	rec := httptest.NewRecorder()
	api.SearchHandler()(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when saturated, got %d", rec.Code)
	}
	close(fr.block)
}

func TestJobNameStatsHandler(t *testing.T) {
	fr := &fakeReader{statRows: []store.JobNameStat{
		{Name: "process-device-action", Queue: "Legacy", State: "Completed", Count: 42},
	}}
	api := NewSearchAPI(fr, 4)

	req := httptest.NewRequest(http.MethodGet, "/v1/stats/job-names?queue=Legacy&state=Completed&since_ms=99&limit=15", nil)
	rec := httptest.NewRecorder()
	api.JobNameStatsHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fr.lastStat.Queue != "Legacy" || fr.lastStat.State != "Completed" ||
		fr.lastStat.SinceMs != 99 || fr.lastStat.Limit != 15 {
		t.Fatalf("params not threaded: %+v", fr.lastStat)
	}
	var out []store.JobNameStat
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(out) != 1 || out[0].Name != "process-device-action" || out[0].Count != 42 {
		t.Fatalf("unexpected body: %+v", out)
	}
}

func TestJobNameStatsHandlerNormalizesGrafanaQueue(t *testing.T) {
	cases := []string{"", "All", "$queue", ".*", "(Legacy|Workflow)", "a,b"}
	for _, q := range cases {
		fr := &fakeReader{}
		api := NewSearchAPI(fr, 4)
		req := httptest.NewRequest(http.MethodGet, "/v1/stats/job-names?queue="+url.QueryEscape(q), nil)
		rec := httptest.NewRecorder()
		api.JobNameStatsHandler()(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("queue %q: expected 200, got %d", q, rec.Code)
		}
		if fr.lastStat.Queue != "" {
			t.Fatalf("queue %q: expected no filter, got %q", q, fr.lastStat.Queue)
		}
	}

	// A plain single queue name passes through as an exact filter.
	fr := &fakeReader{}
	api := NewSearchAPI(fr, 4)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats/job-names?queue=Legacy", nil)
	api.JobNameStatsHandler()(httptest.NewRecorder(), req)
	if fr.lastStat.Queue != "Legacy" {
		t.Fatalf("expected exact queue filter, got %q", fr.lastStat.Queue)
	}
}
