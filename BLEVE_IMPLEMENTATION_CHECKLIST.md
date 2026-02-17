# ✅ Bleve Search Implementation Checklist

Use this to track your 3-5 day implementation sprint.

---

## 📋 Day 1: Infrastructure & Setup

### Morning (2 hours)

- [ ] Read `BLEVE_SEARCH_ANALYSIS.md` sections: Executive Summary + Architecture
- [ ] Review current codebase structure (handlers, explorer, main)
- [ ] Create `internal/search/` directory

```bash
mkdir -p internal/search
touch internal/search/indexer.go
touch internal/search/query.go
touch internal/search/document.go
```

### Midday (4 hours): Bleve Core

**File: `internal/search/document.go`** (Simple data structure)

```go
package search

import "github.com/kofno/bullderdash/internal/explorer"

// SearchableJob is what we index in Bleve
type SearchableJob struct {
    ID           string // Primary key
    JobID        string // Searchable
    Name         string // Job name
    Queue        string // Queue name
    State        string // waiting/active/failed/completed/delayed
    Data         string // Job data as JSON string
    FailedReason string // Error message
    Timestamp    int64  // When created
}

// NewSearchableJob converts Job to SearchableJob
func NewSearchableJob(job *explorer.Job) *SearchableJob {
    // Implementation...
    return &SearchableJob{}
}
```

**File: `internal/search/indexer.go`** (Bleve management)

- [ ] Set up Bleve index creation with proper mapping
- [ ] Implement `IndexJob(job *Job) error`
- [ ] Implement `IndexJobs(jobs []*Job) error` (for batch)
- [ ] Implement `SearchJobs(query string) ([]*Job, error)`
- [ ] Implement `RebuildIndex()` for restart
- [ ] Implement `Close()` for graceful shutdown
- [ ] Add sync.RWMutex for thread safety

```go
package search

import (
    "sync"
    "github.com/blevesearch/bleve/v2"
    "github.com/kofno/bullderdash/internal/explorer"
)

type JobIndexer struct {
    index bleve.Index
    mu    sync.RWMutex
}

func NewJobIndexer() (*JobIndexer, error) {
    // Create mapping
    // Create index
    // Return indexer
}

// Must implement all above methods
```

### Afternoon (2 hours): Integration

**File: `go.mod`**
- [ ] Run `go get github.com/blevesearch/bleve/v2`
- [ ] Verify it's added to go.mod
- [ ] Run `go mod tidy`

**File: `internal/explorer/explorer.go`**
- [ ] Add `indexer *search.JobIndexer` field to Explorer struct
- [ ] Add public method `SearchJobs(query string, queue string) ([]*Job, error)`
- [ ] Update `DiscoverQueues()` to trigger indexing (optional)

**File: `main.go`**
- [ ] Create JobIndexer on startup
- [ ] Build initial index from discovered jobs
- [ ] Pass indexer to Explorer
- [ ] Defer indexer.Close() in cleanup

### Evening (2 hours): Testing Infrastructure

- [ ] Write unit test for indexing
- [ ] Write unit test for basic search
- [ ] Verify tests pass
- [ ] Check test coverage

**✅ Day 1 Goal:** Bleve is working, you can index and search jobs programmatically

---

## 📋 Day 2: API Endpoint & Handler

### Morning (3 hours): Handler Function

**File: `internal/web/handlers.go`**

- [ ] Create `SearchHandler(exp *explorer.Explorer) http.HandlerFunc`
- [ ] Parse query params: `q`, `queue`, `state`
- [ ] Call `exp.SearchJobs()` with filters
- [ ] Format results as JSON
- [ ] Handle errors gracefully
- [ ] Add to metrics tracking

```go
func SearchHandler(exp *explorer.Explorer) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        query := r.URL.Query().Get("q")
        queue := r.URL.Query().Get("queue")
        state := r.URL.Query().Get("state")
        
        // Validate query
        if query == "" {
            http.Error(w, "search query required", 400)
            return
        }
        
        // Call explorer
        results, err := exp.SearchJobs(query, queue)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        
        // Filter by state if specified
        if state != "" {
            results = filterByState(results, state)
        }
        
        // Return JSON
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(results)
    }
}
```

### Midday (2 hours): Query Parser

**File: `internal/search/query.go`**

- [ ] Create `ParseQuery(input string) (BleveQuery, error)`
- [ ] Support simple queries: "email"
- [ ] Support AND/OR: "email AND failed"
- [ ] Support NOT: "-processed"
- [ ] Support phrases: "exact match"
- [ ] Support wildcards: "user*"

```go
package search

import "github.com/blevesearch/bleve/v2/search"

func ParseQuery(input string) (search.Query, error) {
    // Parse user input
    // Convert to Bleve query
    // Return or error
}
```

### Afternoon (2 hours): Route Registration & Testing

**File: `main.go`**
- [ ] Register `/search` route
- [ ] Test with curl/Postman

```go
mux.HandleFunc("GET /search", web.SearchHandler(exp))
```

**Testing:**
- [ ] Test: `/search?q=email` → returns results
- [ ] Test: `/search?q=email&queue=orders` → filtered results
- [ ] Test: `/search?q=` → error
- [ ] Test: `/search?q=nonexistent` → empty results

### Evening (1 hour): Integration Tests

- [ ] Write integration test with real jobs
- [ ] Test search accuracy
- [ ] Test filtering logic
- [ ] Verify performance (<100ms for typical queries)

**✅ Day 2 Goal:** API works, you can search via `/search?q=something`

---

## 📋 Day 3: UI & HTMX Integration

### Morning (3 hours): HTML Templates

**File: `internal/web/handlers.go` (update templates section)**

- [ ] Add search form to main dashboard template
- [ ] Create search results template
- [ ] Style with Tailwind

```html
<!-- Search Form -->
<div class="mb-6 p-4 bg-white rounded-lg shadow">
    <form id="search-form">
        <div class="flex gap-2">
            <input 
                type="text" 
                name="q" 
                placeholder="Search jobs (email, user123, order456...)" 
                class="flex-1 px-4 py-2 border rounded"
                autocomplete="off"
            >
            <select name="queue" class="px-4 py-2 border rounded">
                <option value="">All Queues</option>
                {{range .Queues}}
                <option value="{{.}}">{{.}}</option>
                {{end}}
            </select>
            <select name="state" class="px-4 py-2 border rounded">
                <option value="">All States</option>
                <option value="waiting">Waiting</option>
                <option value="active">Active</option>
                <option value="failed">Failed</option>
                <option value="completed">Completed</option>
                <option value="delayed">Delayed</option>
            </select>
        </div>
    </form>
</div>

<!-- Results -->
<div id="search-results"></div>
```

### Midday (2 hours): Results Template

- [ ] Create results HTML template
- [ ] Show job cards with key info
- [ ] Include click-through to job details
- [ ] Show "no results" message
- [ ] Show search count

```html
{{if .Results}}
<div class="results-container">
    <h2 class="text-xl font-bold mb-4">
        Found {{.Total}} results for "{{.Query}}"
    </h2>
    {{range .Results}}
    <div class="job-result-card mb-4 p-4 border rounded hover:bg-gray-50">
        <div class="flex justify-between">
            <div>
                <p class="font-mono text-sm text-gray-600">{{.ID}}</p>
                <p class="font-bold">{{.Name}}</p>
                <p class="text-sm text-gray-600">Queue: {{.Queue}}</p>
            </div>
            <div class="flex flex-col items-end gap-2">
                <span class="badge badge-{{.State}}">{{.State}}</span>
                <a href="/queue/{{.Queue}}/job/{{.ID}}" class="text-blue-600 hover:underline">
                    View Details →
                </a>
            </div>
        </div>
    </div>
    {{end}}
</div>
{{else}}
<p class="text-gray-600">No results found for "{{.Query}}"</p>
{{end}}
```

### Afternoon (2 hours): HTMX Integration

- [ ] Add HTMX to search form
- [ ] Implement live search with debounce
- [ ] Show loading indicator
- [ ] Update results in real-time

```html
<form 
    hx-get="/search" 
    hx-trigger="keyup delay:500ms from:#search-input"
    hx-target="#search-results"
    hx-request-headers='{"HX-Request": "true"}'
>
    <input 
        id="search-input"
        type="text" 
        name="q" 
        placeholder="Search jobs..."
        autocomplete="off"
    >
    <!-- other fields -->
</form>
```

### Evening (1 hour): Styling & Polish

- [ ] Add CSS for results cards
- [ ] Add loading spinner
- [ ] Add error message styling
- [ ] Add responsive design for mobile
- [ ] Test in browser

**✅ Day 3 Goal:** Search UI works in web interface with live results

---

## 📋 Day 4: Testing & Edge Cases

### Morning (2 hours): Comprehensive Testing

**Test Scenarios:**
- [ ] Single word search: "email"
- [ ] Multiple word search: "email AND failed"
- [ ] Quoted phrase: "must include this"
- [ ] Wildcards: "user*"
- [ ] Negation: "-pending"
- [ ] Empty query (should error)
- [ ] Very long query (should handle or truncate)
- [ ] Special characters: "@", "#", "$"
- [ ] Queue filter only: `queue=orders`
- [ ] State filter only: `state=failed`
- [ ] Combined filters: `queue=orders&state=failed&q=email`
- [ ] Non-ASCII characters: "café", "日本語"
- [ ] Case sensitivity: "Email" vs "email"
- [ ] Partial matches: "user" should match "user123"

**Create: `internal/search/search_test.go`**
```go
package search

import "testing"

func TestIndexJob(t *testing.T) {
    // Test indexing
}

func TestSearchBasic(t *testing.T) {
    // Test simple search
}

func TestSearchFilters(t *testing.T) {
    // Test with queue and state filters
}

// ... more tests
```

### Midday (2 hours): Performance Testing

- [ ] Index 1,000 jobs and measure time
- [ ] Search 1,000 jobs and measure latency
- [ ] Index 10,000 jobs and verify it works
- [ ] Search with complex query
- [ ] Measure memory usage
- [ ] Add metrics to Prometheus

### Afternoon (2 hours): Edge Case Handling

- [ ] Sanitize user input (prevent injection)
- [ ] Handle empty result sets
- [ ] Handle corrupt job data
- [ ] Handle missing fields
- [ ] Handle index rebuild
- [ ] Handle concurrent searches

**✅ Day 4 Goal:** All tests pass, edge cases handled, performance acceptable

---

## 📋 Day 5: Documentation & Polish

### Morning (2 hours): Documentation

- [ ] Document search query syntax
- [ ] Document API endpoint
- [ ] Document configuration options
- [ ] Create usage examples
- [ ] Add to README.md
- [ ] Add troubleshooting section

**Create: `docs/SEARCH_GUIDE.md`**

### Midday (1 hour): Final Polish

- [ ] Code review (check style, naming, comments)
- [ ] Update changelog
- [ ] Verify build still works
- [ ] Test with simulator

### Afternoon (2 hours): Deployment Prep

- [ ] Update docker build if needed
- [ ] Test with docker
- [ ] Test with kubernetes (if applicable)
- [ ] Performance benchmarks documented
- [ ] Ready for production

**✅ Day 5 Goal:** Fully documented, tested, ready for production use

---

## 🎯 Success Criteria

By end of Day 5, you should have:

### Functionality ✅
- [ ] Search works via web UI
- [ ] Search works via API
- [ ] Results are accurate
- [ ] Filters work (queue, state)
- [ ] Performance is acceptable (<100ms per query)

### Quality ✅
- [ ] All tests pass
- [ ] Edge cases handled
- [ ] Error messages are helpful
- [ ] No crashes on bad input
- [ ] Memory usage is reasonable

### Documentation ✅
- [ ] README updated with search info
- [ ] Usage examples provided
- [ ] API documented
- [ ] Troubleshooting guide included

### Production Ready ✅
- [ ] Tested with real jobs
- [ ] Tested with simulator
- [ ] Metrics integrated
- [ ] Performance profiled
- [ ] Ready to deploy

---

## 🚀 Quick Commands

```bash
# Day 1: Setup
mkdir -p internal/search
go get github.com/blevesearch/bleve/v2

# Day 2: Test API
curl "http://localhost:8080/search?q=email"

# Day 3: Test UI
# Open http://localhost:8080 and search in web interface

# Day 4-5: Run tests
go test ./internal/search/...
go test ./internal/web/...

# Full test coverage
go test -cover ./...
```

---

## 📊 Time Allocation

```
Day 1: Infrastructure (8 hours)
  └─ Bleve setup: 4h
  └─ Explorer integration: 2h
  └─ Main initialization: 1h
  └─ Tests: 1h

Day 2: API (8 hours)
  └─ Handler: 3h
  └─ Query parser: 2h
  └─ Route registration: 2h
  └─ Integration tests: 1h

Day 3: UI (8 hours)
  └─ Templates: 3h
  └─ Results view: 2h
  └─ HTMX integration: 2h
  └─ Styling: 1h

Day 4: Testing (8 hours)
  └─ Unit tests: 3h
  └─ Integration tests: 2h
  └─ Performance tests: 2h
  └─ Edge cases: 1h

Day 5: Polish (6 hours)
  └─ Documentation: 2h
  └─ Code review: 1h
  └─ Deployment: 2h
  └─ Final testing: 1h

Total: ~38-40 hours (5 days at 8h/day)
```

---

## 🎓 Helpful References

- Bleve Docs: https://blevesearch.com/
- Bleve Mapping: https://blevesearch.com/docs/Mapping/
- Bleve Queries: https://blevesearch.com/docs/Query-String-Syntax/
- HTMX Docs: https://htmx.org/
- Go JSON: https://golang.org/pkg/encoding/json/

---

## 💡 Pro Tips

1. **Start simple:** Get basic word search working first, add operators later
2. **Use simulator:** Generate test data for performance testing
3. **Commit often:** Daily commits help track progress
4. **Benchmark early:** Know performance characteristics by Day 4
5. **User test:** Get actual users to try search queries
6. **Monitor Prometheus:** Watch query duration metrics

---

## 🔥 When You Get Stuck

**Bleve indexing not working?**
- Check index creation (NewMemOnly vs file-based)
- Verify document fields are set
- Check for errors on Index() call

**Search returns no results?**
- Verify jobs are indexed (count index)
- Check query syntax
- Try simpler query
- Check field names match

**HTMX not triggering?**
- Verify hx-trigger syntax
- Check network tab in browser DevTools
- Ensure endpoint returns HTML

**Performance is slow?**
- Profile with go tool pprof
- Check index size
- Limit results to top 50
- Add caching layer

---

## ✨ You've Got This!

This is straightforward Go work. Follow the checklist, commit daily, and you'll have world-class search by Friday.

**Questions?** Refer back to `BLEVE_SEARCH_ANALYSIS.md` for deep technical details.

Good luck! 🚀

