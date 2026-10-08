package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kofno/bullderdash/internal/store"
)

type fakeReader struct {
	rows    []store.SearchRow
	detail  *store.JobDetail
	getErr  error
	lastP   store.SearchParams
	block   chan struct{} // if set, Search blocks until closed
	entered chan struct{} // signalled when Search is entered
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

func TestSearchHandlerRequiresPredicate(t *testing.T) {
	api := NewSearchAPI(&fakeReader{}, 4)
	req := httptest.NewRequest(http.MethodGet, "/v1/search?since_ms=10&limit=5", nil)
	rec := httptest.NewRecorder()
	api.SearchHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for no predicate, got %d", rec.Code)
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
