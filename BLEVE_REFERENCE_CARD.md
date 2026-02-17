# 📎 Bleve Search: Reference Card

Print this. Tape it to your monitor. You're welcome.

---

## ⏱️ Timeline at a Glance

```
Day 1: Infrastructure         Bleve setup + indexing (8h)
Day 2: API Endpoint           /search handler + parser (8h)  
Day 3: UI + HTMX              Search box + results (8h)
Day 4: Testing & Edge Cases   Comprehensive testing (8h)
Day 5: Documentation & Polish Finish up (6h)
─────────────────────────────────────────────────────
Total: 38-40 hours = 1 work week
```

---

## 📊 Effort Snapshot

| Metric | Value |
|--------|-------|
| Days | 3-5 |
| Lines of Code | ~1,100 |
| New Files | 3 |
| Modified Files | 4 |
| Binary Size Increase | +3-5 MB |
| Runtime Memory Increase | +20-50 MB |
| Complexity | Medium |
| Risk | Very Low |
| Payoff | HUGE |

---

## 🔍 What Bleve Does

```
┌────────────────────────────────────┐
│  User Types: "email"               │
├────────────────────────────────────┤
│  Bleve Searches:                   │
│  - Job names                       │
│  - Job data (JSON)                 │
│  - Error messages                  │
│  - Job IDs                         │
├────────────────────────────────────┤
│  Results: 3 jobs with "email"      │
│  Speed: 5-10ms                     │
│  Accuracy: Exact matches           │
└────────────────────────────────────┘
```

---

## 🎯 Five Files to Create/Modify

```
1. internal/search/indexer.go     (CREATE - 300 lines)
   └─ Manages Bleve index

2. internal/search/query.go        (CREATE - 150 lines)
   └─ Parses user queries

3. internal/web/handlers.go        (MODIFY - +250 lines)
   └─ Add SearchHandler

4. internal/explorer/explorer.go   (MODIFY - +50 lines)
   └─ Add SearchJobs method

5. main.go                         (MODIFY - +40 lines)
   └─ Initialize indexer
```

---

## 💾 Install Bleve

```bash
# That's it
go get github.com/blevesearch/bleve/v2
go mod tidy
```

---

## 🚀 Four Examples of Real Searches

```
Search Query              What Finds                  Use Case
─────────────────────────────────────────────────────────────
email                     Jobs with "email" anywhere  Find email jobs
user123                   Customer ID or job ID       Find customer
queue:orders email        Orders queue with "email"   Scoped search
state:failed bounce       Failed jobs with "bounce"   Find problem jobs
```

---

## 📈 Performance Numbers

```
Query Complexity    Latency     Feel
────────────────────────────────
Simple word         5-10ms      Instant ✨
Two words          10-20ms      Instant ✨
Operators          20-50ms      Instant ✨
Large results      50-200ms     Fast ✅
```

---

## ✅ Success Checklist

- [ ] Bleve dependency added
- [ ] Indexer created and working
- [ ] Explorer integrated
- [ ] API endpoint working
- [ ] Web UI has search form
- [ ] Results display correctly
- [ ] All tests pass
- [ ] Documentation complete
- [ ] Performance acceptable
- [ ] Ready to deploy

---

## 🔴 Red Flags (Shouldn't Happen)

| Problem | Cause | Fix |
|---------|-------|-----|
| No results | Jobs not indexed | Call IndexJobs() on startup |
| Slow search | Index not built | Rebuild index |
| Memory spike | Index too large | Limit indexed fields |
| HTMX not working | Wrong trigger syntax | Check HTML attributes |

---

## 🟢 Green Lights (You're Good)

✅ Index is building
✅ Searches return results in <100ms
✅ Tests pass
✅ UI displays results
✅ You can combine filters
✅ Special characters handled
✅ Memory usage reasonable
✅ Documentation done

---

## 📞 Quick Reference

**Dependency:** `github.com/blevesearch/bleve/v2`  
**Main method:** `indexer.SearchJobs(query, queue)`  
**API endpoint:** `GET /search?q=...&queue=...&state=...`  
**Response:** JSON array of Job objects  
**Speed:** <100ms typical, <500ms worst case  
**Storage:** In-memory (survives app restart, resync on startup)

---

## 🎓 Learning Path

1. **Hour 1:** Read infrastructure section of BLEVE_SEARCH_ANALYSIS.md
2. **Hour 2:** Review Bleve docs at https://blevesearch.com/
3. **Days 1-2:** Implement following checklist
4. **Days 3-5:** Iterate on UI and testing
5. **Day 5:** Document and deploy

---

## 💡 Pro Tips

1. Start with simple queries, add operators later
2. Test with 1,000 jobs first (fast iteration)
3. Use simulator for load testing
4. Commit daily (track progress)
5. Profile performance on day 4
6. Get user feedback early and often

---

## 🚨 When You're Stuck

**"Indexing isn't working"**
→ Check NewMemOnly() call, verify doc fields, check Index() return

**"Search returns nothing"**
→ Verify jobs are in index, try simple single word, check field names

**"HTMX search not triggering"**
→ Check hx-trigger syntax, inspect Network tab, verify endpoint

**"It's slow"**
→ Profile with pprof, check index size, limit results to 100

**"Memory is growing"**
→ Don't index raw job.Opts, limit to essential fields

---

## 📚 The Documents You Have

| Document | Purpose | Length |
|----------|---------|--------|
| BLEVE_SEARCH_ANALYSIS.md | Deep dive | 200+ lines |
| BLEVE_QUICK_START.md | Overview | 150+ lines |
| BLEVE_BOTTOM_LINE.md | Pitch | 120+ lines |
| BLEVE_IMPLEMENTATION_CHECKLIST.md | Day-by-day | 250+ lines |
| BLEVE_EXECUTIVE_SUMMARY.md | For your team | 180+ lines |
| BLEVE_REFERENCE_CARD.md | This one | Quick lookup |

**All in:** `C:\RootDev\bull-der-dash\`

---

## 🎯 Decision

**Should you build Bleve search?**

→ **YES** if:
- Users ask for search
- You have 3-5 days
- You want competitive advantage
- You plan to scale

→ **MAYBE** if:
- You want to validate demand first
- You have less than 3 days
- Start with simple search, upgrade later

→ **LATER** if:
- Other features are higher priority
- Can't commit dedicated time

---

## ⚡ Quick Start Command

```bash
# Day 1 setup
mkdir -p internal/search
go get github.com/blevesearch/bleve/v2
go mod tidy

# Then follow BLEVE_IMPLEMENTATION_CHECKLIST.md
```

---

## 🏁 When You're Done

Users will:
- ✅ Search for jobs by ID
- ✅ Search for jobs by content
- ✅ Find results in <100ms
- ✅ Filter by queue & state
- ✅ Love the speed
- ✅ Ask "why doesn't BullBoard have this?"

You'll have:
- ✅ Competitive advantage
- ✅ Happy users
- ✅ Unique selling point
- ✅ Foundation for future features

---

## 📞 What to Do Now

### Option 1: I'm Ready to Build
→ Open `BLEVE_IMPLEMENTATION_CHECKLIST.md`  
→ Block 5 days  
→ Start Day 1 today  

### Option 2: I Want More Info
→ Read `BLEVE_QUICK_START.md`  
→ Ask me questions  
→ Schedule for later  

### Option 3: Prove It Works First
→ I'll build simple string search (1 day)  
→ You test with users  
→ Upgrade to Bleve based on feedback  

---

**Your choice. Either way, search is coming to Bull-der-dash. 🚀**

