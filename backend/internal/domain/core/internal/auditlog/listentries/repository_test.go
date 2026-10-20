//go:build integration

package listentries

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

func setupDB(t testing.TB) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))); err != nil {
		t.Fatal(err)
	}
	return d
}

type seedRow struct {
	ID, ActorType, ActorID, ActorLabel, ProjectID, ProjectLabel, Action, Target, Status string
	At                                                                                  time.Time
	Args                                                                                string
}

func insert(t testing.TB, d *db.DB, r seedRow) string {
	t.Helper()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.ActorType == "" {
		r.ActorType = "system"
	}
	if r.ActorLabel == "" {
		r.ActorLabel = "system"
	}
	if r.Action == "" {
		r.Action = "x.y"
	}
	if r.Status == "" {
		r.Status = "ok"
	}
	if r.Args == "" {
		r.Args = "{}"
	}
	nul := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	_, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, project_label, action, target, status, args)
		VALUES ($1,$2,$3,$4::uuid,$5,$6::uuid,$7,$8,$9,$10,$11::jsonb)`,
		r.ID, r.At, r.ActorType, nul(r.ActorID), r.ActorLabel, nul(r.ProjectID), nul(r.ProjectLabel), r.Action, nul(r.Target), r.Status, r.Args)
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func ids(es []base.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

func list(t testing.TB, repo Repository, f base.Filter) []base.Entry {
	t.Helper()
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	es, err := repo.List(context.Background(), f, f.Limit)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestRepository_OrderDesc(t *testing.T) {
	d := setupDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 10; i++ {
		insert(t, d, seedRow{At: now.Add(-time.Duration(i*37%10) * time.Minute * 3)})
	}
	es := list(t, NewRepository(d), base.Filter{})
	for i := 1; i < len(es); i++ {
		if es[i].OccurredAt.After(es[i-1].OccurredAt) {
			t.Fatal("tidak urut DESC")
		}
	}
}

func TestRepository_KeysetStableWithEqualTimestamps(t *testing.T) {
	d := setupDB(t)
	repo := NewRepository(d)
	uc := NewUsecase(repo)
	same := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	for i := 0; i < 7; i++ {
		insert(t, d, seedRow{At: same})
	}
	for i := 0; i < 4; i++ {
		insert(t, d, seedRow{At: same.Add(time.Duration(i-2) * time.Minute)})
	}
	all := list(t, repo, base.Filter{Limit: 200})

	var got []string
	f := base.Filter{Limit: 3}
	for page := 0; page < 20; page++ {
		res, err := uc.List(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range res.Items {
			got = append(got, it.ID)
		}
		if res.NextCursor == nil {
			break
		}
		c, err := base.DecodeCursor(*res.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		f.Cursor = &c
		// sisipan lebih baru dari cursor tidak boleh menggeser halaman berikutnya
		if page == 0 {
			insert(t, d, seedRow{At: time.Now().UTC()})
		}
	}
	want := ids(all)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestRepository_CombinedFilterAndBounds(t *testing.T) {
	d := setupDB(t)
	repo := NewRepository(d)
	pA, pB := uuid.NewString(), uuid.NewString()
	from := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	hitFrom := insert(t, d, seedRow{At: from, ProjectID: pA, Action: "mcp.read_file"})
	hitMid := insert(t, d, seedRow{At: from.Add(24 * time.Hour), ProjectID: pA, Action: "mcp.search_knowledge"})
	insert(t, d, seedRow{At: to, ProjectID: pA, Action: "mcp.read_file"})
	insert(t, d, seedRow{At: from.Add(-time.Nanosecond * 1000), ProjectID: pA, Action: "mcp.read_file"})
	insert(t, d, seedRow{At: from.Add(time.Hour), ProjectID: pB, Action: "mcp.read_file"})
	insert(t, d, seedRow{At: from.Add(time.Hour), ProjectID: pA, Action: "auth.login.success"})
	es := list(t, repo, base.Filter{ProjectID: pA, Actions: []string{"mcp.read_file", "mcp.search_knowledge"}, From: from, To: to})
	if fmt.Sprint(ids(es)) != fmt.Sprint([]string{hitMid, hitFrom}) {
		t.Fatalf("got %v", ids(es))
	}
}

func TestRepository_ActorAndStatusAndLabel(t *testing.T) {
	d := setupDB(t)
	repo := NewRepository(d)
	now := time.Now().UTC()
	actor := uuid.NewString()
	mine := insert(t, d, seedRow{At: now, ActorType: "api_key", ActorID: actor, ActorLabel: "claude-code-budi (gk_ab12)"})
	insert(t, d, seedRow{At: now, ActorType: "api_key", ActorID: uuid.NewString(), ActorLabel: "other"})
	if es := list(t, repo, base.Filter{ActorType: "api_key", ActorID: actor}); len(es) != 1 || es[0].ID != mine {
		t.Fatalf("actor: %v", ids(es))
	}
	if es := list(t, repo, base.Filter{ActorLabel: "BUDI"}); len(es) != 1 || es[0].ID != mine {
		t.Fatalf("label: %v", ids(es))
	}
	insert(t, d, seedRow{At: now, Status: "error"})
	insert(t, d, seedRow{At: now, Status: "denied"})
	for _, e := range list(t, repo, base.Filter{Statuses: []string{"ok", "denied"}}) {
		if e.Status == "error" {
			t.Fatal("error ikut")
		}
	}
}

func TestRepository_LikeEscape(t *testing.T) {
	d := setupDB(t)
	repo := NewRepository(d)
	now := time.Now().UTC()
	lit := insert(t, d, seedRow{At: now, Target: "50%_done"})
	insert(t, d, seedRow{At: now, Target: "500 done"})
	insert(t, d, seedRow{At: now, Target: "50x_done"})
	if es := list(t, repo, base.Filter{Target: "50%_"}); len(es) != 1 || es[0].ID != lit {
		t.Fatalf("got %v", ids(es))
	}
	if es := list(t, repo, base.Filter{Target: "%"}); len(es) != 1 {
		t.Fatalf("%% saja harus literal, got %d", len(es))
	}
	if _, err := repo.List(context.Background(), base.Filter{Target: `a\b`}, 5); err != nil {
		t.Fatal(err)
	}
}

func TestRepository_SummaryOnlyAndNulls(t *testing.T) {
	d := setupDB(t)
	repo := NewRepository(d)
	insert(t, d, seedRow{At: time.Now().UTC(), Args: `{"big":"` + string(bytes.Repeat([]byte("x"), 100000)) + `"}`})
	es := list(t, repo, base.Filter{})
	if len(es) != 1 || es[0].Args != nil || es[0].ProjectID != "" || es[0].Target != "" {
		t.Fatalf("%+v", es)
	}
}

func TestRepository_TwentyActions(t *testing.T) {
	d := setupDB(t)
	var acts []string
	for i := 0; i < 20; i++ {
		acts = append(acts, fmt.Sprintf("a.b%d", i))
	}
	insert(t, d, seedRow{At: time.Now().UTC(), Action: "a.b7"})
	if es := list(t, NewRepository(d), base.Filter{Actions: acts}); len(es) != 1 {
		t.Fatalf("got %d", len(es))
	}
}

func superAdminRouter(d *db.DB, role string) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if role != "" {
				req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: "u", Role: role}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Route("/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		Init(app.Deps{DB: d}, r)
	})
	return r
}

func call(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestT02_AC5_Roles(t *testing.T) {
	d := setupDB(t)
	for role, want := range map[string]int{"developer": 403, "": 401, resolvesession.RoleSuperAdmin: 200} {
		if w := call(superAdminRouter(d, role), "GET", "/audit-log"); w.Code != want {
			t.Fatalf("role %q: %d", role, w.Code)
		}
	}
	h := superAdminRouter(d, resolvesession.RoleSuperAdmin)
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if w := call(h, m, "/audit-log"); w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
}

func TestHTTP_PaginationEndToEnd(t *testing.T) {
	d := setupDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		insert(t, d, seedRow{At: now.Add(-time.Duration(i) * time.Minute)})
	}
	h := superAdminRouter(d, resolvesession.RoleSuperAdmin)
	target := "/audit-log?limit=2"
	var sizes []int
	for i := 0; i < 5; i++ {
		w := call(h, "GET", target)
		var res struct {
			Items      []map[string]any `json:"items"`
			NextCursor *string          `json:"next_cursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(res.Items))
		if res.NextCursor == nil {
			break
		}
		target = "/audit-log?limit=2&cursor=" + *res.NextCursor
	}
	if fmt.Sprint(sizes) != "[2 2 1]" {
		t.Fatalf("sizes %v", sizes)
	}
}

func TestT02_AC3_SnapshotLabelsSurviveDeletion(t *testing.T) {
	d := setupDB(t)
	proj, actor := uuid.NewString(), uuid.NewString()
	id := insert(t, d, seedRow{At: time.Now().UTC(), ProjectID: proj, ProjectLabel: "Payments (dihapus)", ActorType: "api_key", ActorID: actor, ActorLabel: "key-lama"})
	w := call(superAdminRouter(d, resolvesession.RoleSuperAdmin), "GET", "/audit-log?project_id="+proj)
	var res Result
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != id || *res.Items[0].ProjectLabel != "Payments (dihapus)" || res.Items[0].ActorLabel != "key-lama" {
		t.Fatalf("%s", w.Body.String())
	}
}
