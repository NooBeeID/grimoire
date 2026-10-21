package renamespec

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/lib/common/reqctx"
)

func serve(g *rig, withID bool, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Patch("/projects/{projectID}/api-specs/{name}", newHandler(g.u).ServeHTTP)
	req := httptest.NewRequest(http.MethodPatch, target, strings.NewReader(body))
	if withID {
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u1", Role: "developer", Name: "Sari"}))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func errBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var b struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b.Error == nil {
		t.Fatalf("body=%s", rec.Body.String())
	}
	return b.Error
}

func TestHandler_OK(t *testing.T) {
	g := newRig()
	rec := serve(g, true, "/projects/"+pid+"/api-specs/payment", `{"name":"payment-v2"}`)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Spec["name"] != "payment-v2" {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if _, ok := body.Spec["source_path"]; !ok {
		t.Fatalf("source_path hilang: %s", rec.Body.String())
	}
	if g.r.renamed[0].Name != "payment-v2" || g.a.events[0].Args["from"] != "payment" {
		t.Fatalf("param tidak diteruskan")
	}
}

func TestHandler_Errors(t *testing.T) {
	path := "/projects/" + pid + "/api-specs/payment"
	t.Run("invalid name", func(t *testing.T) {
		rec := serve(newRig(), true, path, `{"name":"Bad Name"}`)
		e := errBody(t, rec)
		d, _ := e["details"].(map[string]any)
		if rec.Code != 400 || e["code"] != "INVALID_ARGUMENT" || d["reason"] != "INVALID_NAME" {
			t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("missing field", func(t *testing.T) {
		rec := serve(newRig(), true, path, `{}`)
		if rec.Code != 400 {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("not member", func(t *testing.T) {
		g := newRig()
		g.g.err = base.ErrProjectNotFound
		rec := serve(g, true, path, `{"name":"x"}`)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "project tidak ditemukan") {
			t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("spec not found", func(t *testing.T) {
		g := newRig()
		g.r.spec = nil
		rec := serve(g, true, path, `{"name":"x"}`)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "spec tidak ditemukan") {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("conflict", func(t *testing.T) {
		g := newRig()
		g.r.renameErr = errTaken
		rec := serve(g, true, path, `{"name":"other"}`)
		e := errBody(t, rec)
		d, _ := e["details"].(map[string]any)
		if rec.Code != 409 || e["code"] != "CONFLICT" || d["reason"] != "NAME_TAKEN" {
			t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("unauthorized", func(t *testing.T) {
		if rec := serve(newRig(), false, path, `{"name":"x"}`); rec.Code != 401 {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("infra", func(t *testing.T) {
		g := newRig()
		g.r.lockErr = errBoom
		rec := serve(g, true, path, `{"name":"payment-v2"}`)
		if rec.Code != 500 || strings.Contains(rec.Body.String(), "boom") {
			t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
		}
	})
}

var (
	errTaken = store.ErrNameTaken
	errBoom  = errors.New("boom secret")
)
