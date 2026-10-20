package base

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
	MaxActions   = 20
	MaxTextBytes = 200
)

// Filter adalah kriteria daftar audit (semua predikat digabung AND).
type Filter struct {
	ActorType  string
	ActorID    string
	ActorLabel string
	ProjectID  string
	Actions    []string
	Statuses   []string
	From, To   time.Time
	Target     string
	Cursor     *Cursor
	Limit      int
}

func fieldErr(field string) error {
	e := errs.WithDetails(errs.CodeInvalid, field+" tidak valid", map[string]string{"field": field})
	e.Err = errs.ErrInvalid
	return e
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// Validate memeriksa dan menormalisasi filter (default limit, dedupe status).
// Error membungkus errs.ErrInvalid dengan details {"field": nama}.
func (f *Filter) Validate() error {
	switch f.ActorType {
	case "", ActorUser, ActorAPIKey, ActorAnonymous, ActorSystem:
	default:
		return fieldErr("actor_type")
	}
	if f.ActorID != "" && !isUUID(f.ActorID) {
		return fieldErr("actor_id")
	}
	if f.ProjectID != "" && !isUUID(f.ProjectID) {
		return fieldErr("project_id")
	}
	if len(f.Actions) > MaxActions {
		return fieldErr("action")
	}
	for _, a := range f.Actions {
		if !ValidAction(a) {
			return fieldErr("action")
		}
	}
	seen := make(map[string]bool, len(f.Statuses))
	statuses := make([]string, 0, len(f.Statuses))
	for _, s := range f.Statuses {
		switch s {
		case StatusOK, StatusError, StatusDenied:
		default:
			return fieldErr("status")
		}
		if !seen[s] {
			seen[s] = true
			statuses = append(statuses, s)
		}
	}
	if len(f.Statuses) > 0 {
		f.Statuses = statuses
	}
	if !f.From.IsZero() && !f.To.IsZero() && !f.From.Before(f.To) {
		return fieldErr("from")
	}
	f.ActorLabel = strings.TrimSpace(f.ActorLabel)
	f.Target = strings.TrimSpace(f.Target)
	if len(f.ActorLabel) > MaxTextBytes {
		return fieldErr("actor_label")
	}
	if len(f.Target) > MaxTextBytes {
		return fieldErr("target")
	}
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit < 1 || f.Limit > MaxLimit {
		return fieldErr("limit")
	}
	return nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLike meng-escape metakarakter LIKE untuk dipakai dengan ESCAPE '\'.
func EscapeLike(s string) string { return likeEscaper.Replace(s) }
