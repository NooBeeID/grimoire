//go:build integration

package listkeys

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

type httpEnv struct {
	d   *db.DB
	p2  string
	r   chi.Router
	acl map[string]checkaccess.Level // userID|projectID
}

func setupHTTP(t *testing.T, busErr error) (*httpEnv, string) {
	t.Helper()
	d := setupDB(t)
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	e := &httpEnv{acl: map[string]checkaccess.Level{}}
	if busErr != errNoHandler {
		b.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
			if busErr != nil {
				return nil, busErr
			}
			in := req.(checkaccess.Request)
			if _, err := uuid.Parse(in.ProjectID); err != nil {
				return checkaccess.Response{}, nil
			}
			if in.UserRole == "super_admin" {
				return checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}, nil
			}
			lv, ok := e.acl[in.UserID+"|"+in.ProjectID]
			if !ok {
				lv = checkaccess.LevelNone
			}
			return checkaccess.Response{ProjectExists: true, Level: lv}, nil
		})
	}
	root := chi.NewRouter()
	api := chi.NewRouter()
	root.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := r.Header.Get("X-Test-User"); u != "" {
				role := "developer"
				if strings.HasPrefix(u, "S") {
					role = "super_admin"
				}
				r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: u, Role: role, Email: u + "@tim.id"}))
			}
			next.ServeHTTP(w, r)
		})
	})
	Init(app.Deps{DB: d, Bus: b, API: api})
	root.Mount("/api", api)
	e.r = root
	p1 := mkProject(t, d, "P1")
	p2 := mkProject(t, d, "P2")
	e.acl["A|"+p1] = checkaccess.LevelOwner
	e.acl["B|"+p1] = checkaccess.LevelMember
	e.d = d
	e.p2 = p2
	return e, p1
}

var errNoHandler = errors.New("no handler")

func (e *httpEnv) get(user, pid string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+pid+"/api-keys", nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	e.r.ServeHTTP(rec, req)
	return rec
}

func TestT04_AC1_AccessMatrix(t *testing.T) {
	e, p1 := setupHTTP(t, nil)
	for _, u := range []string{"A", "B", "S1"} {
		if rec := e.get(u, p1); rec.Code != 200 {
			t.Fatalf("%s: %d %s", u, rec.Code, rec.Body)
		}
	}
	nf := e.get("C", p1)
	if nf.Code != 404 || !strings.Contains(nf.Body.String(), "project tidak ditemukan") {
		t.Fatalf("non member: %d %s", nf.Code, nf.Body)
	}
	if rec := e.get("C", "bukan-uuid"); rec.Code != 404 {
		t.Fatalf("non-uuid: %d", rec.Code)
	}
	if rec := e.get("", p1); rec.Code != 401 {
		t.Fatalf("unauth: %d", rec.Code)
	}
}

func TestListKeys_CheckAccessFailureIs500(t *testing.T) {
	for name, berr := range map[string]error{"error": errors.New("boom"), "unregistered": errNoHandler} {
		t.Run(name, func(t *testing.T) {
			e, p1 := setupHTTP(t, berr)
			rec := e.get("A", p1)
			if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"INTERNAL"`) || strings.Contains(rec.Body.String(), "boom") {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestListKeys_EmptyProject(t *testing.T) {
	e, p1 := setupHTTP(t, nil)
	rec := e.get("A", p1)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"items":[]}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestT04_AC2_NoSecretAndOrderingAndIsolation(t *testing.T) {
	e, p1 := setupHTTP(t, nil)
	base := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	rev := base.Add(5 * time.Hour)
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(0xA0 + i)
	}
	old := mkKey(t, e.d, p1, "old", "ddddddd1", base, nil, hash)
	cur := mkKey(t, e.d, p1, "new", "ddddddd2", base.Add(time.Hour), nil, nil)
	gone := mkKey(t, e.d, p1, "gone", "ddddddd3", base.Add(2*time.Hour), &rev, nil)
	other := mkKey(t, e.d, e.p2, "other", "ddddddd4", base, nil, nil)

	rec := e.get("B", p1)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	raw := rec.Body.String()
	for _, bad := range []string{"secret", "hash", "raw_key", "a0a1a2a3", "oKGio6Sl", other, "other"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("leak %q in %s", bad, raw)
		}
	}
	var res struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{cur, old, gone}
	if len(res.Items) != 3 {
		t.Fatalf("items=%d", len(res.Items))
	}
	keys := []string{"id", "name", "prefix", "status", "created_by", "created_by_label", "created_at", "last_used_at", "revoked_at", "revoked_by", "revoked_by_label"}
	for i, it := range res.Items {
		if it["id"] != wantIDs[i] || len(it) != len(keys) {
			t.Fatalf("item %d: %v", i, it)
		}
		for _, k := range keys {
			if _, ok := it[k]; !ok {
				t.Fatalf("missing %s", k)
			}
		}
	}
	if res.Items[2]["status"] != "revoked" || res.Items[2]["revoked_by_label"] != "a@tim.id" ||
		res.Items[2]["revoked_at"] != "2026-10-23T15:00:00Z" || res.Items[0]["status"] != "active" || res.Items[0]["revoked_at"] != nil {
		t.Fatalf("status fields: %v", res.Items)
	}
	// A bukan anggota P2 -> 404 seragam.
	if rec := e.get("A", e.p2); rec.Code != 404 {
		t.Fatalf("P2: %d", rec.Code)
	}
	// keanggotaan dicabut -> 404, A tetap 200.
	delete(e.acl, "B|"+p1)
	if e.get("B", p1).Code != 404 || e.get("A", p1).Code != 200 {
		t.Fatal("membership removal")
	}
}
