// Package base berisi entity dan business rule audit log tanpa I/O.
package base

import (
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
)

// Nilai enum dire-ekspor dari kontrak bus agar feature cukup meng-import base.
const (
	ActorUser      = record.ActorUser
	ActorAPIKey    = record.ActorAPIKey
	ActorAnonymous = record.ActorAnonymous
	ActorSystem    = record.ActorSystem

	StatusOK     = record.StatusOK
	StatusError  = record.StatusError
	StatusDenied = record.StatusDenied
)

// Batas panjang (byte) kolom teks.
const (
	MaxActionBytes       = 128
	MaxActorLabelBytes   = 256
	MaxProjectLabelBytes = 256
	MaxTargetTypeBytes   = 64
	MaxTargetBytes       = record.MaxTargetBytes
	MaxUserAgentBytes    = 512
	MaxTraceIDBytes      = 128
)

var actionRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// ValidAction melaporkan apakah s berformat aksi audit (<area>.<objek>[.<aksi>]).
func ValidAction(s string) bool {
	return len(s) <= MaxActionBytes && actionRe.MatchString(s)
}

// Entry adalah satu entri audit yang sudah tervalidasi dan tersanitasi.
// String kosong berarti NULL untuk kolom nullable (dikonversi di repository).
type Entry struct {
	ID           string
	OccurredAt   time.Time
	ActorType    string
	ActorID      string
	ActorLabel   string
	ProjectID    string
	ProjectLabel string
	Action       string
	TargetType   string
	Target       string
	Args         map[string]any
	Result       map[string]any
	Status       string
	IP           string
	UserAgent    string
	TraceID      string
}

func invalid(field string) error {
	return errs.Wrap(errs.CodeInvalid, field+" tidak valid", errs.ErrInvalid)
}

func canonicalUUID(s string) (string, bool) {
	u, err := uuid.Parse(s)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

func normalizeIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		ap, err2 := netip.ParseAddrPort(s)
		if err2 != nil {
			return ""
		}
		addr = ap.Addr()
	}
	return addr.WithZone("").Unmap().String()
}

// NewEntry memvalidasi dan menormalisasi request audit menjadi Entry.
// Error membungkus errs.ErrInvalid dan tidak memuat nilai input.
func NewEntry(in record.Request, now time.Time, newID func() string) (Entry, error) {
	e := Entry{
		Action:       strings.TrimSpace(in.Action),
		ActorType:    strings.TrimSpace(in.ActorType),
		ActorID:      strings.TrimSpace(in.ActorID),
		ActorLabel:   TruncateUTF8(strings.TrimSpace(in.ActorLabel), MaxActorLabelBytes),
		ProjectID:    strings.TrimSpace(in.ProjectID),
		ProjectLabel: TruncateUTF8(strings.TrimSpace(in.ProjectLabel), MaxProjectLabelBytes),
		TargetType:   TruncateUTF8(strings.TrimSpace(in.TargetType), MaxTargetTypeBytes),
		Target:       TruncateUTF8(strings.TrimSpace(in.Target), MaxTargetBytes),
		Status:       strings.TrimSpace(in.Status),
		UserAgent:    TruncateUTF8(strings.TrimSpace(in.UserAgent), MaxUserAgentBytes),
		TraceID:      TruncateUTF8(strings.TrimSpace(in.TraceID), MaxTraceIDBytes),
		IP:           normalizeIP(in.IP),
	}

	if !ValidAction(e.Action) {
		return Entry{}, invalid("action")
	}

	switch e.ActorType {
	case ActorUser, ActorAPIKey:
		id, ok := canonicalUUID(e.ActorID)
		if !ok {
			return Entry{}, invalid("actor_id")
		}
		e.ActorID = id
		if e.ActorLabel == "" {
			return Entry{}, invalid("actor_label")
		}
	case ActorAnonymous, ActorSystem:
		if e.ActorID != "" {
			return Entry{}, invalid("actor_id")
		}
		if e.ActorLabel == "" {
			e.ActorLabel = e.ActorType
		}
	default:
		return Entry{}, invalid("actor_type")
	}

	if e.ProjectID != "" {
		id, ok := canonicalUUID(e.ProjectID)
		if !ok {
			return Entry{}, invalid("project_id")
		}
		e.ProjectID = id
	}

	switch e.Status {
	case StatusOK, StatusError, StatusDenied:
	default:
		return Entry{}, invalid("status")
	}

	e.OccurredAt = in.OccurredAt
	if e.OccurredAt.IsZero() {
		e.OccurredAt = now
	}
	e.OccurredAt = e.OccurredAt.UTC()

	if id := strings.TrimSpace(in.ID); id != "" {
		cid, ok := canonicalUUID(id)
		if !ok {
			return Entry{}, invalid("id")
		}
		e.ID = cid
	} else {
		e.ID = newID()
	}

	e.Args = Sanitize(in.Args)
	e.Result = Sanitize(in.Result)
	return e, nil
}

// Summary adalah representasi ringkas entri untuk daftar (tanpa args/result).
type Summary struct {
	ID           string    `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ActorType    string    `json:"actor_type"`
	ActorID      *string   `json:"actor_id"`
	ActorLabel   string    `json:"actor_label"`
	ProjectID    *string   `json:"project_id"`
	ProjectLabel *string   `json:"project_label"`
	Action       string    `json:"action"`
	TargetType   *string   `json:"target_type"`
	Target       *string   `json:"target"`
	Status       string    `json:"status"`
}

// Detail adalah representasi penuh satu entri.
type Detail struct {
	Summary
	Args      map[string]any `json:"args"`
	Result    map[string]any `json:"result"`
	IP        *string        `json:"ip"`
	UserAgent *string        `json:"user_agent"`
	TraceID   string         `json:"trace_id"`
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ToSummary mengubah Entry menjadi Summary.
func (e Entry) ToSummary() Summary {
	return Summary{
		ID:           e.ID,
		OccurredAt:   e.OccurredAt.UTC(),
		ActorType:    e.ActorType,
		ActorID:      nullable(e.ActorID),
		ActorLabel:   e.ActorLabel,
		ProjectID:    nullable(e.ProjectID),
		ProjectLabel: nullable(e.ProjectLabel),
		Action:       e.Action,
		TargetType:   nullable(e.TargetType),
		Target:       nullable(e.Target),
		Status:       e.Status,
	}
}

// ToDetail mengubah Entry menjadi Detail.
func (e Entry) ToDetail() Detail {
	args, result := e.Args, e.Result
	if args == nil {
		args = map[string]any{}
	}
	if result == nil {
		result = map[string]any{}
	}
	return Detail{
		Summary:   e.ToSummary(),
		Args:      args,
		Result:    result,
		IP:        nullable(e.IP),
		UserAgent: nullable(e.UserAgent),
		TraceID:   e.TraceID,
	}
}
