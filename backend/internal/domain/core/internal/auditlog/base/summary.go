package base

import (
	"math"
	"sort"
	"strings"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

// Range preset ringkasan.
type Range string

const (
	Range24h     Range = "24h"
	Range7d      Range = "7d"
	Range30d     Range = "30d"
	DefaultRange       = Range7d
)

// Bucket adalah satuan date_trunc yang di-whitelist (dipakai sebagai literal parameter SQL).
type Bucket string

const (
	BucketHour Bucket = "hour"
	BucketDay  Bucket = "day"
)

// Interval mengembalikan teks interval Postgres untuk generate_series.
func (b Bucket) Interval() string {
	if b == BucketHour {
		return "1 hour"
	}
	return "1 day"
}

// Step mengembalikan durasi satu bucket.
func (b Bucket) Step() time.Duration {
	if b == BucketHour {
		return time.Hour
	}
	return 24 * time.Hour
}

// SummaryQuery adalah input ringkasan yang sudah diparse (bentuk).
type SummaryQuery struct {
	Range     Range
	ProjectID string
}

// Validate menormalisasi Range kosong ke 7d, menolak range/project_id tidak valid,
// dan menurunkan ProjectID ke huruf kecil.
func (q *SummaryQuery) Validate() error {
	switch q.Range {
	case "":
		q.Range = DefaultRange
	case Range24h, Range7d, Range30d:
	default:
		return fieldErr("range")
	}
	if q.ProjectID != "" {
		if !isUUID(q.ProjectID) {
			return fieldErr("project_id")
		}
		q.ProjectID = strings.ToLower(q.ProjectID)
	}
	return nil
}

// Window adalah rentang [From, To) dan bucket trafiknya.
type Window struct {
	Range  Range
	From   time.Time
	To     time.Time
	Bucket Bucket
}

// NewWindow menyusun window mundur dari now. Prasyarat: r sudah tervalidasi.
func NewWindow(r Range, now time.Time) Window {
	to := now.UTC().Truncate(time.Second)
	w := Window{Range: r, To: to, Bucket: BucketDay}
	switch r {
	case Range24h:
		w.From = to.Add(-24 * time.Hour)
		w.Bucket = BucketHour
	case Range30d:
		w.From = to.Add(-720 * time.Hour)
	default:
		w.From = to.Add(-168 * time.Hour)
	}
	return w
}

// BucketStarts mengembalikan awal setiap bucket dari truncate(From) sampai bucket terakhir < To.
func (w Window) BucketStarts() []time.Time {
	step := w.Bucket.Step()
	var out []time.Time
	for t := w.From.UTC().Truncate(step); t.Before(w.To); t = t.Add(step) {
		out = append(out, t)
	}
	return out
}

// GatewayActions adalah aksi protokol/gateway yang bukan panggilan tool.
func GatewayActions() []string {
	return []string{
		record.ActionMCPInitialize, record.ActionMCPToolsList, record.ActionMCPAuthRejected,
		record.ActionMCPRateLimited, record.ActionMCPUnknownTool,
	}
}

// ToolActions = record.Actions() berawalan "mcp." dikurangi GatewayActions(), urutan katalog.
func ToolActions() []string {
	gw := make(map[string]bool)
	for _, a := range GatewayActions() {
		gw[a] = true
	}
	out := []string{}
	for _, a := range record.Actions() {
		if strings.HasPrefix(a, "mcp.") && !gw[a] {
			out = append(out, a)
		}
	}
	return out
}

// ToolName membuang awalan "mcp.".
func ToolName(action string) string { return strings.TrimPrefix(action, "mcp.") }

// ToolAgg adalah satu baris agregat per aksi tool.
type ToolAgg struct {
	Action string
	Calls  int
	OK     int
	Errors int
	P50Ms  *float64
	P95Ms  *float64
}

// ProjectAgg adalah satu baris top project.
type ProjectAgg struct {
	ProjectID    string
	ProjectLabel string
	Calls        int
}

// TrafficPoint adalah satu bucket trafik.
type TrafficPoint struct {
	At     time.Time `json:"at"`
	Calls  int       `json:"calls"`
	Errors int       `json:"errors"`
}

// Aggregates adalah hasil mentah repository dari satu snapshot.
type Aggregates struct {
	Tools    []ToolAgg
	Projects []ProjectAgg
	Traffic  []TrafficPoint
}

// TopProjectsLimit adalah jumlah maksimum top project.
const TopProjectsLimit = 10

type TopProject struct {
	ProjectID    string `json:"project_id"`
	ProjectLabel string `json:"project_label"`
	Calls        int    `json:"calls"`
}

type ToolUsage struct {
	Tool  string `json:"tool"`
	Calls int    `json:"calls"`
}

type ToolMetric struct {
	Tool      string   `json:"tool"`
	Calls     int      `json:"calls"`
	OK        int      `json:"ok"`
	Error     int      `json:"error"`
	ErrorRate float64  `json:"error_rate"`
	P50Ms     *float64 `json:"p50_ms"`
	P95Ms     *float64 `json:"p95_ms"`
}

type SearchQuality struct {
	Available      bool                       `json:"available"`
	ErrorCode      string                     `json:"error_code,omitempty"`
	Searches       *int                       `json:"searches,omitempty"`
	WindowSeconds  *int                       `json:"window_seconds,omitempty"`
	ZeroResult     *searchquality.Metric      `json:"zero_result,omitempty"`
	FollowUp       *searchquality.Metric      `json:"follow_up,omitempty"`
	Reformulation  *searchquality.Metric      `json:"reformulation,omitempty"`
	RankOpened     []searchquality.RankBucket `json:"rank_opened,omitempty"`
	LegacySearches *int                       `json:"legacy_searches,omitempty"`
}

const SearchQualityUnavailable = "UNAVAILABLE"

// QualityFromReport memetakan Report T11 1:1 (Available=true).
func QualityFromReport(r searchquality.Report) *SearchQuality {
	searches, ws, legacy := r.Searches, r.WindowSeconds, r.LegacySearches
	zr, fu, rf := r.ZeroResult, r.FollowUp, r.Reformulation
	return &SearchQuality{
		Available:      true,
		Searches:       &searches,
		WindowSeconds:  &ws,
		ZeroResult:     &zr,
		FollowUp:       &fu,
		Reformulation:  &rf,
		RankOpened:     append([]searchquality.RankBucket(nil), r.RankOpened...),
		LegacySearches: &legacy,
	}
}

// QualityUnavailable mengembalikan {Available:false, ErrorCode:"UNAVAILABLE"}.
func QualityUnavailable() *SearchQuality {
	return &SearchQuality{Available: false, ErrorCode: SearchQualityUnavailable}
}

type Report struct {
	Range         Range          `json:"range"`
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	Bucket        Bucket         `json:"bucket"`
	ProjectID     *string        `json:"project_id"`
	TotalCalls    int            `json:"total_calls"`
	TotalErrors   int            `json:"total_errors"`
	ErrorRate     *float64       `json:"error_rate"`
	TopProjects   []TopProject   `json:"top_projects"`
	ToolUsage     []ToolUsage    `json:"tool_usage"`
	ToolMetrics   []ToolMetric   `json:"tool_metrics"`
	Traffic       []TrafficPoint `json:"traffic"`
	SearchQuality *SearchQuality `json:"search_quality"`
}

func round2(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := math.Round(*p*100) / 100
	return &v
}

// BuildReport menyusun laporan dari agregat (lihat plan langkah a).
func BuildReport(w Window, q SummaryQuery, a Aggregates, sq *SearchQuality) (Report, error) {
	starts := w.BucketStarts()
	if len(a.Traffic) != len(starts) {
		return Report{}, errs.New(errs.CodeInternal, "summary: jumlah bucket trafik tidak sesuai")
	}
	traffic := make([]TrafficPoint, len(a.Traffic))
	for i, p := range a.Traffic {
		if !p.At.Equal(starts[i]) {
			return Report{}, errs.New(errs.CodeInternal, "summary: awal bucket trafik tidak sesuai")
		}
		p.At = p.At.UTC()
		traffic[i] = p
	}

	metrics := make([]ToolMetric, 0, len(a.Tools))
	total, totalErr := 0, 0
	for _, t := range a.Tools {
		m := ToolMetric{
			Tool: ToolName(t.Action), Calls: t.Calls, OK: t.OK, Error: t.Errors,
			P50Ms: round2(t.P50Ms), P95Ms: round2(t.P95Ms),
		}
		if t.Calls > 0 {
			m.ErrorRate = float64(t.Errors) / float64(t.Calls)
		}
		total += t.Calls
		totalErr += t.Errors
		metrics = append(metrics, m)
	}
	sort.SliceStable(metrics, func(i, j int) bool {
		if metrics[i].Calls != metrics[j].Calls {
			return metrics[i].Calls > metrics[j].Calls
		}
		return metrics[i].Tool < metrics[j].Tool
	})
	usage := make([]ToolUsage, len(metrics))
	for i, m := range metrics {
		usage[i] = ToolUsage{Tool: m.Tool, Calls: m.Calls}
	}

	top := make([]TopProject, 0, len(a.Projects))
	for _, p := range a.Projects {
		label := p.ProjectLabel
		if label == "" {
			label = p.ProjectID
		}
		top = append(top, TopProject{ProjectID: p.ProjectID, ProjectLabel: label, Calls: p.Calls})
	}

	r := Report{
		Range: w.Range, From: w.From, To: w.To, Bucket: w.Bucket,
		TotalCalls: total, TotalErrors: totalErr,
		TopProjects: top, ToolUsage: usage, ToolMetrics: metrics, Traffic: traffic,
		SearchQuality: sq,
	}
	if q.ProjectID != "" {
		id := q.ProjectID
		r.ProjectID = &id
	}
	if total > 0 {
		rate := float64(totalErr) / float64(total)
		r.ErrorRate = &rate
	}
	return r, nil
}
