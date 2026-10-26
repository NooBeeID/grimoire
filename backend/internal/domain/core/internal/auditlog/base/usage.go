package base

import (
	"strings"
	"time"

	"grimoire/lib/contracts/core/audit/record"
)

// Section adalah nama bagian insight pemakaian agent (nilai parameter `section`).
type Section string

const (
	SectionTopFiles    Section = "top_files"
	SectionUnreadFiles Section = "unread_files"
	SectionZeroQueries Section = "zero_queries"
	SectionKeys        Section = "keys"
)

// Batas jumlah baris per bagian dan batas pemindaian file.
const (
	TopFilesLimit    = 20
	UnreadLimit      = 50
	ZeroQueriesLimit = 20
	PerKeyLimit      = 20
	MaxScanFiles     = 2000
)

// Kode galat bagian.
const (
	SectionUnavailable     = "UNAVAILABLE"
	SectionProjectRequired = "PROJECT_REQUIRED"
)

// ReadActions adalah aksi pembacaan dokumen oleh agent.
func ReadActions() []string {
	return []string{record.ActionMCPReadFile, record.ActionMCPReadSection}
}

// NeedsProject: bagian yang path-nya hanya bermakna per project.
func (s Section) NeedsProject() bool {
	return s == SectionTopFiles || s == SectionUnreadFiles
}

// UsageQuery adalah input insight yang sudah diparse (bentuk).
type UsageQuery struct {
	Range     Range
	ProjectID string
	Section   Section
}

// Validate menormalisasi range/project_id seperti SummaryQuery dan memeriksa section.
func (q *UsageQuery) Validate() error {
	sq := SummaryQuery{Range: q.Range, ProjectID: q.ProjectID}
	if err := sq.Validate(); err != nil {
		return err
	}
	q.Range, q.ProjectID = sq.Range, sq.ProjectID
	switch q.Section {
	case "", SectionTopFiles, SectionUnreadFiles, SectionZeroQueries, SectionKeys:
	default:
		return fieldErr("section")
	}
	return nil
}

// Wants: true bila bagian s diminta (section kosong = semua).
func (q UsageQuery) Wants(s Section) bool { return q.Section == "" || q.Section == s }

type FileRead struct {
	Path       string    `json:"path"`
	Reads      int       `json:"reads"`
	Keys       int       `json:"keys"`
	LastReadAt time.Time `json:"last_read_at"`
}

type UnreadFile struct {
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ZeroQuery struct {
	Query  string    `json:"query"`
	Count  int       `json:"count"`
	Keys   int       `json:"keys"`
	LastAt time.Time `json:"last_at"`
}

type KeyUsage struct {
	KeyID      string    `json:"key_id"`
	Label      string    `json:"label"`
	Calls      int       `json:"calls"`
	Errors     int       `json:"errors"`
	Searches   int       `json:"searches"`
	Reads      int       `json:"reads"`
	LastUsedAt time.Time `json:"last_used_at"`
	TopTool    string    `json:"top_tool"`
}

// KnownFile adalah file project yang dikenal (dari bus list-files).
type KnownFile struct {
	Path      string
	UpdatedAt time.Time
}

type TopFilesSection struct {
	Available bool       `json:"available"`
	ErrorCode string     `json:"error_code,omitempty"`
	Items     []FileRead `json:"items"`
}

type UnreadFilesSection struct {
	Available bool         `json:"available"`
	ErrorCode string       `json:"error_code,omitempty"`
	Items     []UnreadFile `json:"items"`
	Truncated bool         `json:"truncated"`
}

type ZeroQueriesSection struct {
	Available bool        `json:"available"`
	ErrorCode string      `json:"error_code,omitempty"`
	Items     []ZeroQuery `json:"items"`
}

type PerKeySection struct {
	Available bool       `json:"available"`
	ErrorCode string     `json:"error_code,omitempty"`
	Items     []KeyUsage `json:"items"`
}

func capped[T any](items []T, n int) []T {
	if len(items) > n {
		items = items[:n]
	}
	out := make([]T, len(items))
	copy(out, items)
	return out
}

func TopFilesOK(items []FileRead) *TopFilesSection {
	return &TopFilesSection{Available: true, Items: capped(items, TopFilesLimit)}
}

func ZeroQueriesOK(items []ZeroQuery) *ZeroQueriesSection {
	return &ZeroQueriesSection{Available: true, Items: capped(items, ZeroQueriesLimit)}
}

// PerKeyOK membuang awalan "mcp." dari TopTool.
func PerKeyOK(items []KeyUsage) *PerKeySection {
	out := capped(items, PerKeyLimit)
	for i := range out {
		out[i].TopTool = ToolName(out[i].TopTool)
	}
	return &PerKeySection{Available: true, Items: out}
}

// BuildUnread memilih path unread yang masih ada di files, mengikuti urutan unread.
func BuildUnread(files []KnownFile, unread []string, scanCapped bool) *UnreadFilesSection {
	known := make(map[string]time.Time, len(files))
	for _, f := range files {
		known[f.Path] = f.UpdatedAt
	}
	valid := make([]UnreadFile, 0, len(unread))
	for _, p := range unread {
		if at, ok := known[p]; ok {
			valid = append(valid, UnreadFile{Path: p, UpdatedAt: at.UTC()})
		}
	}
	return &UnreadFilesSection{
		Available: true,
		Items:     capped(valid, UnreadLimit),
		Truncated: len(valid) > UnreadLimit || scanCapped,
	}
}

func TopFilesError(code string) *TopFilesSection {
	return &TopFilesSection{ErrorCode: code, Items: []FileRead{}}
}

func UnreadFilesError(code string) *UnreadFilesSection {
	return &UnreadFilesSection{ErrorCode: code, Items: []UnreadFile{}}
}

func ZeroQueriesError(code string) *ZeroQueriesSection {
	return &ZeroQueriesSection{ErrorCode: code, Items: []ZeroQuery{}}
}

func PerKeyError(code string) *PerKeySection {
	return &PerKeySection{ErrorCode: code, Items: []KeyUsage{}}
}

// UsageReport adalah respons /audit-log/usage; bagian tak diminta bernilai nil.
type UsageReport struct {
	Range       Range               `json:"range"`
	From        time.Time           `json:"from"`
	To          time.Time           `json:"to"`
	ProjectID   *string             `json:"project_id"`
	TopFiles    *TopFilesSection    `json:"top_files"`
	UnreadFiles *UnreadFilesSection `json:"unread_files"`
	ZeroQueries *ZeroQueriesSection `json:"zero_queries"`
	PerKey      *PerKeySection      `json:"per_key"`
}

// NewUsageReport mengisi header laporan; semua bagian nil.
func NewUsageReport(w Window, q UsageQuery) UsageReport {
	r := UsageReport{Range: w.Range, From: w.From, To: w.To}
	if q.ProjectID != "" {
		id := strings.ToLower(q.ProjectID)
		r.ProjectID = &id
	}
	return r
}
