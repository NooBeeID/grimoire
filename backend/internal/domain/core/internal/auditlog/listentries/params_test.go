package listentries

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

func parseValidate(raw string) (base.Filter, error) {
	q, _ := url.ParseQuery(raw)
	f, err := ParseFilter(q)
	if err != nil {
		return f, err
	}
	return f, f.Validate()
}

func fieldOf(err error) string {
	var e *errs.Error
	if errors.As(err, &e) {
		if m, ok := e.Details.(map[string]string); ok {
			return m["field"]
		}
	}
	return ""
}

func TestParseFilter_Empty(t *testing.T) {
	f, err := parseValidate("")
	if err != nil {
		t.Fatal(err)
	}
	if f.Limit != 50 || f.Cursor != nil || !f.From.IsZero() {
		t.Fatalf("unexpected: %+v", f)
	}
}

func TestParseFilter_RepeatedParams(t *testing.T) {
	f, err := parseValidate("action=mcp.read_file&action=mcp.search_knowledge&status=ok&status=denied&entry=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Actions, []string{"mcp.read_file", "mcp.search_knowledge"}) {
		t.Fatalf("actions: %v", f.Actions)
	}
	if !reflect.DeepEqual(f.Statuses, []string{"ok", "denied"}) {
		t.Fatalf("statuses: %v", f.Statuses)
	}
}

func TestParseFilter_TimeOffsetToUTC(t *testing.T) {
	f, err := parseValidate("from=2026-10-20T07:00:00%2B07:00&to=2026-10-21T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !f.From.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)) || f.From.Location() != time.UTC {
		t.Fatalf("from: %v", f.From)
	}
}

func TestParseFilter_Invalid(t *testing.T) {
	cursorOK := base.EncodeCursor(base.Cursor{OccurredAt: time.Now(), ID: uuid.NewString()})
	_ = cursorOK
	long := strings.Repeat("x", 201)
	var acts []string
	for i := 0; i < 21; i++ {
		acts = append(acts, "action=a.b")
	}
	cases := []struct{ name, q, field string }{
		{"from bukan tanggal", "from=kemarin", "from"},
		{"to bukan rfc3339", "to=2026-10-20", "to"},
		{"limit abc", "limit=abc", "limit"},
		{"limit 201", "limit=201", "limit"},
		{"limit -1", "limit=-1", "limit"},
		{"cursor rusak", "cursor=rusak!", "cursor"},
		{"actor_type", "actor_type=admin", "actor_type"},
		{"actor_id", "actor_id=abc", "actor_id"},
		{"project_id", "project_id=1", "project_id"},
		{"action", "action=BAD", "action"},
		{"action kosong", "action=", "action"},
		{"status", "status=x", "status"},
		{"from >= to", "from=2026-10-21T00:00:00Z&to=2026-10-20T00:00:00Z", "from"},
		{"21 action", strings.Join(acts, "&"), "action"},
		{"target panjang", "target=" + long, "target"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseValidate(c.q)
			if !errors.Is(err, errs.ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
			if got := fieldOf(err); got != c.field {
				t.Fatalf("field = %q, want %q", got, c.field)
			}
		})
	}
}

func TestParseFilter_ErrorDoesNotEchoInput(t *testing.T) {
	_, err := parseValidate("cursor=rusak!")
	if err == nil || strings.Contains(err.Error(), "rusak!") {
		t.Fatalf("err = %v", err)
	}
	_, err = parseValidate("from=kemarin")
	if err == nil || strings.Contains(err.Error(), "kemarin") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseFilter_LimitBoundaries(t *testing.T) {
	for raw, want := range map[string]int{"limit=0": 50, "limit=1": 1, "limit=200": 200} {
		f, err := parseValidate(raw)
		if err != nil || f.Limit != want {
			t.Fatalf("%s: %v %d", raw, err, f.Limit)
		}
	}
}

func TestParseFilter_ValidCursor(t *testing.T) {
	at := time.Date(2026, 10, 19, 8, 14, 22, 123456789, time.UTC)
	id := uuid.NewString()
	f, err := parseValidate("cursor=" + base.EncodeCursor(base.Cursor{OccurredAt: at, ID: id}))
	if err != nil {
		t.Fatal(err)
	}
	if f.Cursor == nil || !f.Cursor.OccurredAt.Equal(at) || f.Cursor.ID != id {
		t.Fatalf("cursor: %+v", f.Cursor)
	}
}
