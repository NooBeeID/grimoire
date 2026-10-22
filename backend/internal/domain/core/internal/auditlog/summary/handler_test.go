package summary

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
)

type fakeUC struct {
	called int
	got    base.SummaryQuery
	res    base.Report
	err    error
}

func (f *fakeUC) Summarize(_ context.Context, q base.SummaryQuery) (base.Report, error) {
	f.called++
	f.got = q
	return f.res, f.err
}

func get(uc summarizer, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/summary", NewHandler(uc).Get)
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
		"/summary?range=1d":                "range",
		"/summary?range=90d":               "range",
		"/summary?range=7D":                "range",
		"/summary?project_id=abc":          "project_id",
		"/summary?range=7d&project_id=xyz": "project_id",
	} {
		uc := &fakeUC{}
		w := get(uc, target)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", target, w.Code)
		}
		e := errDetail(t, w)
		if e["code"] != "INVALID_ARGUMENT" {
			t.Fatalf("%s: code %v", target, e["code"])
		}
		d, _ := e["details"].(map[string]any)
		if d["field"] != field {
			t.Fatalf("%s: field %v", target, d["field"])
		}
		if uc.called != 0 {
			t.Fatalf("%s: usecase dipanggil", target)
		}
	}
}

func TestHandler_InternalErrorGeneric(t *testing.T) {
	w := get(&fakeUC{err: errors.New("pq: relation audit_log secret sql")}, "/summary")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "secret sql") {
		t.Fatalf("membocorkan error: %s", w.Body.String())
	}
	if errDetail(t, w)["code"] != "INTERNAL" {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestHandler_OKShapeAndNormalization(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	w0 := base.NewWindow(base.Range7d, now)
	rep, err := base.BuildReport(w0, base.SummaryQuery{Range: base.Range7d}, base.Aggregates{Traffic: trafficFor(w0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	uc := &fakeUC{res: rep}
	w := get(uc, "/summary?project_id=0192F5A0-1111-7AAA-8BBB-000000000001")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	if uc.got.Range != base.Range7d || uc.got.ProjectID != "0192f5a0-1111-7aaa-8bbb-000000000001" {
		t.Fatalf("query ternormalisasi salah: %+v", uc.got)
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
	want := []string{"bucket", "error_rate", "from", "project_id", "range", "search_quality", "to", "tool_metrics", "tool_usage", "top_projects", "total_calls", "total_errors", "traffic"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("keys %v", keys)
	}
	for _, k := range []string{"top_projects", "tool_usage", "tool_metrics"} {
		if string(m[k]) != "[]" {
			t.Fatalf("%s = %s, ingin []", k, m[k])
		}
	}
}

func trafficFor(w base.Window) []base.TrafficPoint {
	var out []base.TrafficPoint
	for _, s := range w.BucketStarts() {
		out = append(out, base.TrafficPoint{At: s})
	}
	return out
}
