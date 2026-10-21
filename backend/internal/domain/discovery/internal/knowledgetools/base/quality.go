package base

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

// Batas dan default laporan kualitas pencarian (T11).
const (
	DefaultQualityWindow  = 120 * time.Second
	MinQualityWindow      = time.Second
	MaxQualityWindow      = time.Hour
	MaxQualityScanEntries = 2_000_000
)

// AuditEvent adalah entri audit mcp.search_knowledge / mcp.read_file yang sudah dibaca.
type AuditEvent struct {
	ID, ActorID, Action, Target, Status string
	At                                  time.Time
	Query                               string // args.query (search saja)
	Total                               int
	HasTotal                            bool     // result.total terbaca sebagai angka
	Paths                               []string // result.result_paths
	HasPaths                            bool     // kunci result_paths ada dan berbentuk array string
}

// EventFromAudit membaca entri audit mentah. Tipe tak terduga dianggap tidak ada; tidak panic.
func EventFromAudit(id, action, actorID, target, status string, at time.Time, args, result map[string]any) AuditEvent {
	e := AuditEvent{ID: id, Action: action, ActorID: actorID, Target: target, Status: status, At: at}
	if q, ok := args["query"].(string); ok {
		e.Query = q
	}
	if n, ok := toInt(result["total"]); ok {
		e.Total, e.HasTotal = n, true
	}
	if raw, ok := result["result_paths"]; ok {
		switch v := raw.(type) {
		case []string:
			e.Paths, e.HasPaths = append([]string{}, v...), true
		case []any:
			paths := make([]string, 0, len(v))
			valid := true
			for _, it := range v {
				s, ok := it.(string)
				if !ok {
					valid = false
					break
				}
				paths = append(paths, s)
			}
			if valid {
				e.Paths, e.HasPaths = paths, true
			}
		}
	}
	return e
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
			return int(i), true
		}
		if f, err := n.Float64(); err == nil {
			return int(f), true
		}
	}
	return 0, false
}

// NormalizeQuery membandingkan query tanpa beda kapital dan spasi berlebih.
func NormalizeQuery(q string) string {
	return strings.ToLower(strings.Join(strings.Fields(q), " "))
}

// QualityReport adalah laporan kualitas pencarian.
type QualityReport = searchquality.Report

const maxRank = MaxResultPaths

// openSearch adalah search yang jendelanya masih terbuka.
type openSearch struct {
	at         time.Time
	evaluated  bool
	hasPaths   bool
	paths      []string
	query      string
	followDone bool // read yang cocok sudah ditemukan
	readSeen   bool // ada read dalam jendela (tak cocok)
	closed     bool
}

// QualityAccumulator menghitung metrik dari event berurut (At, ID) ASC secara streaming.
type QualityAccumulator struct {
	since, until time.Time
	window       time.Duration

	open map[string][]*openSearch // per aktor, urut waktu
	last map[string]*openSearch   // search terakhir per aktor (kandidat reformulasi)

	searches, legacy       int
	zeroCount, zeroDen     int
	followCount, followDen int
	reformCount            int
	rank                   [maxRank + 1]int // indeks 1..maxRank
	notInResults           int
}

func NewQualityAccumulator(since, until time.Time, window time.Duration) *QualityAccumulator {
	if window <= 0 {
		window = DefaultQualityWindow
	}
	return &QualityAccumulator{
		since: since, until: until, window: window,
		open: map[string][]*openSearch{}, last: map[string]*openSearch{},
	}
}

// Add WAJIB dipanggil urut (At, ID) ASC. Event di luar [since, until+window) diabaikan.
func (a *QualityAccumulator) Add(e AuditEvent) {
	if e.Status != record.StatusOK || e.ActorID == "" {
		return
	}
	if e.At.Before(a.since) || !e.At.Before(a.until.Add(a.window)) {
		return
	}
	a.evict(e.At)
	switch e.Action {
	case record.ActionMCPSearchKnowledge:
		a.addSearch(e)
	case record.ActionMCPReadFile:
		a.addRead(e)
	}
}

func (a *QualityAccumulator) addSearch(e AuditEvent) {
	// Reformulasi: search ok pertama aktor sama setelah search sebelumnya.
	if prev := a.last[e.ActorID]; prev != nil && prev.evaluated {
		if d := e.At.Sub(prev.at); d > 0 && d <= a.window && NormalizeQuery(prev.query) != NormalizeQuery(e.Query) {
			a.reformCount++
		}
	}
	s := &openSearch{at: e.At, hasPaths: e.HasPaths, paths: e.Paths, query: e.Query}
	s.evaluated = !e.At.Before(a.since) && e.At.Before(a.until)
	a.last[e.ActorID] = s
	if !s.evaluated {
		return
	}
	a.searches++
	if e.HasTotal {
		a.zeroDen++
		if e.Total == 0 {
			a.zeroCount++
		}
	}
	if !e.HasPaths {
		a.legacy++
		return
	}
	a.followDen++
	a.open[e.ActorID] = append(a.open[e.ActorID], s)
}

func (a *QualityAccumulator) addRead(e AuditEvent) {
	for _, s := range a.open[e.ActorID] {
		d := e.At.Sub(s.at)
		if s.closed || s.followDone || d <= 0 || d > a.window {
			continue
		}
		s.readSeen = true
		for i, p := range s.paths {
			if p == e.Target {
				s.followDone = true
				a.followCount++
				if i+1 <= maxRank {
					a.rank[i+1]++
				}
				break
			}
		}
	}
}

// evict menutup search terbuka yang jendelanya sudah lewat pada waktu now.
func (a *QualityAccumulator) evict(now time.Time) {
	for actor, list := range a.open {
		kept := list[:0]
		for _, s := range list {
			if now.Sub(s.at) > a.window {
				a.finalize(s)
				continue
			}
			kept = append(kept, s)
		}
		if len(kept) == 0 {
			delete(a.open, actor)
		} else {
			a.open[actor] = kept
		}
	}
	for actor, s := range a.last {
		if now.Sub(s.at) > a.window {
			delete(a.last, actor)
		}
	}
}

func (a *QualityAccumulator) finalize(s *openSearch) {
	if s.closed {
		return
	}
	s.closed = true
	if s.readSeen && !s.followDone {
		a.notInResults++
	}
}

func rate(c, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(c) / float64(d)
}

// Report menutup semua search terbuka lalu mengembalikan laporan. Rate dihitung di sini.
func (a *QualityAccumulator) Report(projectID string) QualityReport {
	for _, list := range a.open {
		for _, s := range list {
			a.finalize(s)
		}
	}
	a.open = map[string][]*openSearch{}
	buckets := make([]searchquality.RankBucket, 0, maxRank+1)
	for i := 1; i <= maxRank; i++ {
		buckets = append(buckets, searchquality.RankBucket{Label: strconv.Itoa(i), Count: a.rank[i]})
	}
	buckets = append(buckets, searchquality.RankBucket{Label: searchquality.NotInResults, Count: a.notInResults})
	metric := func(c, d int) searchquality.Metric {
		return searchquality.Metric{Count: c, Denominator: d, Rate: rate(c, d)}
	}
	return QualityReport{
		ProjectID: projectID, Since: a.since, Until: a.until,
		WindowSeconds:  int(a.window / time.Second),
		Searches:       a.searches,
		ZeroResult:     metric(a.zeroCount, a.zeroDen),
		FollowUp:       metric(a.followCount, a.followDen),
		Reformulation:  metric(a.reformCount, a.searches),
		RankOpened:     buckets,
		LegacySearches: a.legacy,
	}
}
