package createkey

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/common/reqctx"
)

func serve(t *testing.T, r *rig, body, ua string) *httptest.ResponseRecorder {
	t.Helper()
	mux := chi.NewRouter()
	mux.Post("/projects/{projectID}/api-keys", newHandler(r.uc).ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/"+pid+"/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "user-b", Email: "b@tim.id", Role: "developer"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandler_Created(t *testing.T) {
	r := newRig("https://g.tim.id", 20)
	rec := serve(t, r, `{"name":"claude-code-budi"}`, "")
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	key := m["key"].(map[string]any)
	if len(m["raw_key"].(string)) != 55 || m["mcp_url"] != "https://g.tim.id/mcp" ||
		key["last_used_at"] != nil || key["status"] != "active" || key["created_at"] != "2026-10-23T09:00:00Z" {
		t.Fatalf("%s", rec.Body.String())
	}
	if r.ac.calls != 1 {
		t.Fatal("access harus dipanggil dengan projectID dari URL")
	}
}

func TestHandler_BadBody_400_StillNoStore(t *testing.T) {
	for _, b := range []string{`{"nama":"x"}`, ``, `{bad`, `{"name":12}`} {
		r := newRig("", 20)
		rec := serve(t, r, b, "")
		if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%q: %d", b, rec.Code)
		}
		if r.rp.txCalls != 0 || strings.Contains(rec.Body.String(), "raw_key") {
			t.Fatalf("%q", b)
		}
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		rec := serve(t, newRig("", 20), `{"name":"a"}`, "")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"name"`) || !strings.Contains(rec.Body.String(), "trace_id") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("not found", func(t *testing.T) {
		r := newRig("", 20)
		r.ac.err = base.ErrProjectNotFound
		rec := serve(t, r, `{"name":"ci"}`, "")
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "project tidak ditemukan") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("name taken", func(t *testing.T) {
		r := newRig("", 20)
		r.rp.insertErrs = []error{base.ErrNameTaken}
		rec := serve(t, r, `{"name":"ci"}`, "")
		if rec.Code != 409 || !strings.Contains(rec.Body.String(), "NAME_TAKEN") ||
			!strings.Contains(rec.Body.String(), "nama key sudah dipakai key aktif Anda yang lain di project ini") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("limit", func(t *testing.T) {
		r := newRig("", 20)
		r.rp.count = 20
		rec := serve(t, r, `{"name":"ci"}`, "")
		if rec.Code != 409 || !strings.Contains(rec.Body.String(), "KEY_LIMIT_REACHED") || !strings.Contains(rec.Body.String(), `"limit":20`) ||
			!strings.Contains(rec.Body.String(), "batas 20 key aktif per user per project tercapai; cabut key yang tidak dipakai") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("internal", func(t *testing.T) {
		r := newRig("", 20)
		r.rp.countErr = http.ErrAbortHandler
		rec := serve(t, r, `{"name":"ci"}`, "")
		if rec.Code != 500 || strings.Contains(rec.Body.String(), "abort") || strings.Contains(rec.Body.String(), "raw_key") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandler_UserAgentTruncated(t *testing.T) {
	r := newRig("", 20)
	serve(t, r, `{"name":"ci"}`, strings.Repeat("é", 400))
	if len(r.au.events) != 1 || len(r.au.events[0].Meta.UserAgent) > 512 || r.au.events[0].Meta.UserAgent == "" {
		t.Fatalf("%+v", r.au.events)
	}
}
