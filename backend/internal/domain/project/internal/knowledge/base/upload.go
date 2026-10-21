package base

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusUpdated   Status = "updated"
	StatusUnchanged Status = "unchanged"
	StatusSkipped   Status = "skipped"
	StatusRejected  Status = "rejected"
)

const maxRawNameRunes = 1024

type Incoming struct {
	Index   int    // urutan di request (0-based)
	Path    string // sudah tervalidasi
	Content string
	Hash    []byte
}

type Decision struct {
	Index  int
	Path   string // path tervalidasi, atau nama mentah (dipotong 1024 rune) bila path ditolak
	Status Status
	Reason Reason // "" untuk created/updated/unchanged
}

func rawDisplayName(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if utf8.RuneCountInString(s) > maxRawNameRunes {
		s = string([]rune(s)[:maxRawNameRunes])
	}
	return s
}

// ClassifyPart menerapkan rule per file, berurutan: path invalid -> rejected; tersembunyi -> skipped hidden;
// tipe tidak didukung -> rejected; tooLarge -> rejected too_large; isi invalid -> rejected. Lolos -> ok=true.
func ClassifyPart(index int, rawName string, raw []byte, tooLarge bool, maxBytes int64) (Incoming, Decision, bool) {
	p, reason := NormalizePath(rawName)
	if reason != "" {
		return Incoming{}, Decision{Index: index, Path: rawDisplayName(rawName), Status: StatusRejected, Reason: reason}, false
	}
	if IsHidden(p) {
		return Incoming{}, Decision{Index: index, Path: p, Status: StatusSkipped, Reason: ReasonHidden}, false
	}
	if !IsSupported(p) {
		return Incoming{}, Decision{Index: index, Path: p, Status: StatusRejected, Reason: ReasonUnsupportedType}, false
	}
	if tooLarge {
		return Incoming{}, Decision{Index: index, Path: p, Status: StatusRejected, Reason: ReasonTooLarge}, false
	}
	content, reason := PrepareContent(raw, maxBytes)
	if reason != "" {
		return Incoming{}, Decision{Index: index, Path: p, Status: StatusRejected, Reason: reason}, false
	}
	return Incoming{Index: index, Path: p, Content: content, Hash: Hash(content)}, Decision{}, true
}

// PlanUpload memutuskan nasib file yang lolos ClassifyPart, diproses urut Index. existing = hash tersimpan per path.
func PlanUpload(in []Incoming, existing map[string][]byte, currentCount, projectLimit int) []Decision {
	sorted := append([]Incoming(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })

	seen := make(map[string]bool, len(sorted))
	created := 0
	out := make([]Decision, 0, len(sorted))
	for _, f := range sorted {
		d := Decision{Index: f.Index, Path: f.Path}
		old, exists := existing[f.Path]
		switch {
		case seen[f.Path]:
			d.Status, d.Reason = StatusRejected, ReasonDuplicateInRequest
		case exists && bytes.Equal(old, f.Hash):
			d.Status = StatusUnchanged
		case exists:
			d.Status = StatusUpdated
		case currentCount+created >= projectLimit:
			d.Status, d.Reason = StatusRejected, ReasonProjectFileLimit
		default:
			d.Status = StatusCreated
			created++
		}
		seen[f.Path] = true
		out = append(out, d)
	}
	return out
}

type Summary struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Rejected  int `json:"rejected"`
}

func Summarize(ds []Decision) Summary {
	var s Summary
	for _, d := range ds {
		switch d.Status {
		case StatusCreated:
			s.Created++
		case StatusUpdated:
			s.Updated++
		case StatusUnchanged:
			s.Unchanged++
		case StatusSkipped:
			s.Skipped++
		case StatusRejected:
			s.Rejected++
		}
	}
	return s
}

// ChangedPaths mengembalikan path created+updated urut Index, maksimal n; truncated bila ada yang terpotong.
func ChangedPaths(ds []Decision, n int) (paths []string, truncated bool) {
	sorted := append([]Decision(nil), ds...)
	SortByIndex(sorted)
	paths = []string{}
	for _, d := range sorted {
		if d.Status != StatusCreated && d.Status != StatusUpdated {
			continue
		}
		if len(paths) >= n {
			return paths, true
		}
		paths = append(paths, d.Path)
	}
	return paths, false
}

// SortByIndex mengurutkan keputusan gabungan (pra-tolak + rencana) sesuai urutan request.
func SortByIndex(ds []Decision) {
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].Index < ds[j].Index })
}

type UploadItemDTO struct {
	Path   string `json:"path"`
	Status Status `json:"status"`
	Reason Reason `json:"reason,omitempty"`
}

type UploadResultDTO struct {
	Summary Summary         `json:"summary"`
	Items   []UploadItemDTO `json:"items"`
}

// NewUploadResult membangun laporan; item diurutkan menurut Index (salinan, input tidak diubah).
func NewUploadResult(ds []Decision) UploadResultDTO {
	sorted := append([]Decision(nil), ds...)
	SortByIndex(sorted)
	items := make([]UploadItemDTO, 0, len(sorted))
	for _, d := range sorted {
		items = append(items, UploadItemDTO{Path: d.Path, Status: d.Status, Reason: d.Reason})
	}
	return UploadResultDTO{Summary: Summarize(sorted), Items: items}
}
