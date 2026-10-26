package usage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

type fakeUC struct {
	called int
	got    base.UsageQuery
	res    base.UsageReport
	err    error
}

func (f *fakeUC) Usage(_ context.Context, q base.UsageQuery) (base.UsageReport, error) {
	f.called++
	f.got = q
	return f.res, f.err
}

func get(uc usager, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/usage", NewHandler(uc).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func errDetail(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	e, _ := m["error"].(map[string]any)
	return e
}

func TestHandler_Invalid(t *testing.T) {
	for target, field := range map[string]string{
		"/usage?range=1y":           "range",
		"/usage?project_id=x":       "project_id",
		"/usage?section=files":      "section",
		"/usage?section=KEYS":       "section",
		"/usage?range=7d&section=a": "section",
	} {
		uc := &fakeUC{}
		w := get(uc, target)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", target, w.Code)
		}
		e := errDetail(t, w)
		d, _ := e["details"].(map[string]any)
		if e["code"] != "INVALID_ARGUMENT" || d["field"] != field {
			t.Fatalf("%s: %v", target, e)
		}
		if uc.called != 0 {
			t.Fatalf("%s: usecase dipanggil", target)
		}
	}
}

func TestHandler_NotFound(t *testing.T) {
	w := get(&fakeUC{err: errs.Wrap(errs.CodeNotFound, "project tidak ditemukan", errs.ErrNotFound)}, "/usage?project_id=0192f5a0-1111-7aaa-8bbb-000000000001")
	if w.Code != http.StatusNotFound || errDetail(t, w)["code"] != "NOT_FOUND" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_InternalErrorGeneric(t *testing.T) {
	w := get(&fakeUC{err: errors.New("pq: secret sql")}, "/usage")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret sql") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_OK(t *testing.T) {
	win := base.NewWindow(base.Range7d, time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC))
	uc := &fakeUC{res: base.NewUsageReport(win, base.UsageQuery{Range: base.Range7d})}
	w := get(uc, "/usage?project_id=0192F5A0-1111-7AAA-8BBB-000000000001&section=keys")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if uc.got.Range != base.Range7d || uc.got.ProjectID != "0192f5a0-1111-7aaa-8bbb-000000000001" || uc.got.Section != base.SectionKeys {
		t.Fatalf("query: %+v", uc.got)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "from,per_key,project_id,range,to,top_files,unread_files,zero_queries"
	if strings.Join(keys, ",") != want {
		t.Fatalf("keys %v", keys)
	}
}
