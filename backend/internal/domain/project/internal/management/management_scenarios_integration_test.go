//go:build integration

package management

// Skenario HTTP + DB + bus lintas feature dari plan T03 yang belum tercakup
// test per feature: pencarian kandidat, race/CSRF/fail-open update, ubah role,
// hapus anggota, tambah anggota, dan no-audit untuk fitur baca.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/contentcount"
)

// ---- helper ----

// rawDo seperti do, tetapi header CSRF bisa dihilangkan dan sesi opsional.
func (e *env) rawDo(uid, method, path string, body string, csrf bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set("X-Requested-With", "grimoire")
	}
	if uid != "" {
		req.Header.Set("X-Test-Uid", uid)
		req.Header.Set("X-Test-Role", e.roleOf(uid))
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

// both menjalankan dua fungsi serentak dari garis start yang sama.
func both(a, b func()) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; a() }()
	go func() { defer wg.Done(); <-start; b() }()
	close(start)
	wg.Wait()
}

func (e *env) roleInDB(pid, uid string) string {
	e.t.Helper()
	var r string
	err := e.d.GetContext(context.Background(), &r, `SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`, pid, uid)
	if err != nil {
		return ""
	}
	return r
}

func (e *env) projectExists(pid string) bool {
	e.t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id = $1`, pid); err != nil {
		e.t.Fatal(err)
	}
	return n == 1
}

func (e *env) execSQL(q string, args ...any) {
	e.t.Helper()
	if _, err := e.d.ExecContext(context.Background(), q, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) orphanMembers() int {
	e.t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members m WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = m.project_id)`); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// normBody menghapus trace_id agar body error bisa dibandingkan.
func normBody(w *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	delete(m, "trace_id")
	out, _ := json.Marshal(m)
	return string(out)
}

func (e *env) finderCalls() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]int, 0, len(e.finderReqs))
	for _, r := range e.finderReqs {
		out = append(out, r.Limit)
	}
	return out
}

type candidateItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (e *env) candidates(uid, pid, query string) []candidateItem {
	e.t.Helper()
	w := e.do(uid, http.MethodGet, "/projects/"+pid+"/member-candidates"+query, nil)
	mustCode(e.t, w, 200)
	var out struct {
		Items []candidateItem `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	if out.Items == nil {
		e.t.Fatalf("items harus array, bukan null: %s", w.Body.String())
	}
	return out.Items
}

func candIDs(items []candidateItem) []string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	sort.Strings(ids)
	return ids
}

func containsID(items []candidateItem, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func routePresent(t *testing.T, e *env, want string) {
	t.Helper()
	found := false
	err := chi.Walk(e.top, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method+" "+strings.TrimSuffix(route, "/") == want {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("route %s tidak terdaftar", want)
	}
}

// ---- searchmembercandidates ----

func TestT03_SearchCandidates_OwnerExcludesExistingMembers(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	got := e.candidates(e.A, p.ID, "?q=x.id")
	if containsID(got, e.A) || containsID(got, e.B) {
		t.Fatalf("anggota ikut sebagai kandidat: %+v", got)
	}
	if !containsID(got, e.C) {
		t.Fatalf("C harus kandidat: %+v", got)
	}
	if containsID(got, e.D) || containsID(got, e.S) {
		t.Fatalf("nonaktif/admin tidak boleh muncul: %+v", got)
	}
	for _, it := range got {
		if it.Name == "" || it.Email == "" {
			t.Fatalf("%+v", it)
		}
	}
}

func TestT03_SearchCandidates_EmptyQuery(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	for _, q := range []string{"", "?q=", "?q=%20%20%20"} {
		if got := e.candidates(e.A, p.ID, q); len(got) != 0 {
			t.Fatalf("q=%q: %+v", q, got)
		}
	}
	if n := len(e.finderCalls()); n != 0 {
		t.Fatalf("provider dipanggil %d kali", n)
	}
	w := e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates", nil)
	if strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestT03_SearchCandidates_RespectsLimit(t *testing.T) {
	e := setup(t)
	for i := 0; i < 5; i++ {
		e.addUser(uuid.NewString(), "Extra"+string(rune('a'+i)), roleDev, true)
	}
	p := e.create(e.A, "Alpha")
	if got := e.candidates(e.A, p.ID, "?q=x.id&limit=2"); len(got) != 2 {
		t.Fatalf("limit=2 -> %d", len(got))
	}
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=x.id&limit=50", nil), 200)
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=x.id&limit=51", nil), 400)
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=x.id&limit=0", nil), 400)
}

func TestT03_SearchCandidates_ProviderCalledWithMax50(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	e.candidates(e.A, p.ID, "?q=%20%20car%20&limit=3")
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.finderReqs) != 1 || e.finderReqs[0].Limit != 50 || e.finderReqs[0].Query != "car" {
		t.Fatalf("%+v", e.finderReqs)
	}
}

func TestT03_SearchCandidates_ProviderError(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	e.finderFail.Store(true)
	w := e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil)
	mustCode(t, w, 500)
	if strings.Contains(w.Body.String(), "finder down") {
		t.Fatalf("pesan asal bocor: %s", w.Body.String())
	}
}

func TestT03_SearchCandidates_ProviderMissing(t *testing.T) {
	e := setupWith(t, setupOpts{noFinder: true})
	p := e.create(e.A, "Alpha")
	w := e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil)
	mustCode(t, w, 500)
	if parseErr(w).Error.Code == "" || strings.Contains(strings.ToLower(w.Body.String()), "not registered") {
		t.Fatalf("%s", w.Body.String())
	}
}

func TestT03_SearchCandidates_SuperAdminSees(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	got := e.candidates(e.S, p.ID, "?q=car")
	if len(got) != 1 || got[0].ID != e.C {
		t.Fatalf("%+v", got)
	}
}

func TestT03_SearchCandidates_Unauthenticated(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.do("", http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil), 401)
}

func TestT03_SearchCandidates_NoAudit(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	before := len(e.auditList())
	e.candidates(e.A, p.ID, "?q=car")
	mustCode(t, e.do(e.B, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil), 403)
	mustCode(t, e.do(e.C, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil), 404)
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car&limit=0", nil), 400)
	if after := len(e.auditList()); after != before {
		t.Fatalf("audit bertambah: %d -> %d", before, after)
	}
}

func TestT03_SearchCandidates_AfterAddMemberDisappears(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	if !containsID(e.candidates(e.A, p.ID, "?q=car"), e.C) {
		t.Fatal("C harus muncul")
	}
	e.execSQL(`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'member')`, p.ID, e.C)
	if containsID(e.candidates(e.A, p.ID, "?q=car"), e.C) {
		t.Fatal("C masih muncul setelah jadi anggota")
	}
}

func TestT03_Security_SearchCandidates_Isolation(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	var first string
	for _, id := range []string{p.ID, uuid.NewString(), "bukan-uuid"} {
		for _, q := range []string{"?q=car", "?q=car&limit=999"} {
			w := e.do(e.C, http.MethodGet, "/projects/"+id+"/member-candidates"+q, nil)
			b := parseErr(w)
			if w.Code != 404 || b.Error.Code != "NOT_FOUND" || b.Error.Message != "project tidak ditemukan" || b.Error.Details != nil {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if first == "" {
				first = normBody(w)
			} else if normBody(w) != first {
				t.Fatalf("body tidak seragam: %s vs %s", normBody(w), first)
			}
		}
	}
	if n := len(e.finderCalls()); n != 0 {
		t.Fatalf("provider dipanggil %d kali", n)
	}
	w := e.do(e.A, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=car", nil)
	for _, bad := range []string{"password", "hash", "role", "is_active"} {
		if strings.Contains(strings.ToLower(w.Body.String()), bad) {
			t.Fatalf("kebocoran %q: %s", bad, w.Body.String())
		}
	}
}

// ---- updateproject ----

func TestUpdate_RouteRegistered(t *testing.T) {
	e := setup(t)
	routePresent(t, e, "PATCH /api/projects/{projectID}")
	routePresent(t, e, "GET /api/projects/{projectID}")
	routePresent(t, e, "DELETE /api/projects/{projectID}")
}

func TestUpdate_CSRFAndAuth(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	w := e.rawDo(e.A, http.MethodPatch, "/projects/"+p.ID, `{"name":"Hacked"}`, false)
	mustCode(t, w, 403)
	if parseErr(w).Error.Code != "CSRF_REQUIRED" {
		t.Fatalf("%s", w.Body.String())
	}
	mustCode(t, e.do("", http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Hacked"}), 401)
	if d := e.detail(e.A, p.ID); d.Name != "Alpha" {
		t.Fatalf("nama berubah: %s", d.Name)
	}
}

func TestUpdate_ConcurrentPatches(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Alpha")
		var c1, c2 int
		both(
			func() { c1 = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Xx"}).Code },
			func() {
				c2 = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"description": "Y"}).Code
			},
		)
		if c1 != 200 || c2 != 200 {
			t.Fatalf("putaran %d: %d %d", i, c1, c2)
		}
		if d := e.detail(e.A, p.ID); d.Name != "Xx" || d.Description != "Y" {
			t.Fatalf("putaran %d lost update: %+v", i, d)
		}
	}
	if n := len(e.auditActions(record.ActionProjectUpdated)); n != 40 {
		t.Fatalf("audit update %d, want 40", n)
	}
}

func TestUpdate_ConcurrentSameField(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		e.mu.Lock()
		e.audits = nil
		e.mu.Unlock()
		p := e.create(e.A, "Alpha")
		e.mu.Lock()
		e.audits = nil
		e.mu.Unlock()
		var c1, c2 int
		both(
			func() { c1 = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "N1"}).Code },
			func() { c2 = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "N2"}).Code },
		)
		if c1 != 200 || c2 != 200 {
			t.Fatalf("%d %d", c1, c2)
		}
		final := e.detail(e.A, p.ID).Name
		if final != "N1" && final != "N2" {
			t.Fatalf("nama akhir %q", final)
		}
		as := e.auditActions(record.ActionProjectUpdated)
		if len(as) != 2 {
			t.Fatalf("audit %d", len(as))
		}
		var first, second *record.Request
		for i := range as {
			if as[i].Args["name_from"] == "Alpha" {
				first = &as[i]
			} else {
				second = &as[i]
			}
		}
		if first == nil || second == nil || second.Args["name_from"] != first.Args["name_to"] {
			t.Fatalf("rantai audit tidak konsisten: %+v", as)
		}
		if second.Args["name_to"] != final {
			t.Fatalf("name_to terakhir %v != %s", second.Args["name_to"], final)
		}
	}
}

func TestUpdate_RaceWithDelete(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Alpha")
		var code int
		both(
			func() { code = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Beta"}).Code },
			func() { e.execSQL(`DELETE FROM projects WHERE id = $1`, p.ID) },
		)
		if code != 200 && code != 404 {
			t.Fatalf("putaran %d: status %d", i, code)
		}
	}
	if n := e.orphanMembers(); n != 0 {
		t.Fatalf("baris yatim: %d", n)
	}
}

func TestUpdate_AuditFailOpen(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	e.auditFail.Store(true)
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Beta"}), 200)
	if d := e.detail(e.A, p.ID); d.Name != "Beta" {
		t.Fatalf("data tidak tersimpan: %s", d.Name)
	}
	e2 := setupWith(t, setupOpts{noAudit: true})
	p2 := e2.create(e2.A, "Alpha")
	mustCode(t, e2.do(e2.A, http.MethodPatch, "/projects/"+p2.ID, map[string]string{"name": "Beta"}), 200)
	if d := e2.detail(e2.A, p2.ID); d.Name != "Beta" {
		t.Fatalf("data tidak tersimpan: %s", d.Name)
	}
}

func TestUpdate_Noop200NoAudit(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	before := e.detail(e.A, p.ID)
	n := len(e.auditList())
	w := e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Alpha"})
	mustCode(t, w, 200)
	var got base.ProjectDetail
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at berubah: %v -> %v", before.UpdatedAt, got.UpdatedAt)
	}
	if after := e.detail(e.A, p.ID); !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("updated_at di DB berubah")
	}
	if len(e.auditList()) != n {
		t.Fatal("no-op tidak boleh mengaudit")
	}
}

func TestT03_Counts_FailSoft_Update(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	w := e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Beta"})
	mustCode(t, w, 200)
	var d base.ProjectDetail
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	if d.Counts.Files != nil || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v", d.Counts)
	}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, func(context.Context, any) (any, error) {
		return contentcount.Response{Counts: map[string]int{p.ID: 3}}, nil
	})
	w = e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Gamma"})
	mustCode(t, w, 200)
	d = base.ProjectDetail{}
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	if d.Counts.Files == nil || *d.Counts.Files != 3 || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v", d.Counts)
	}
}

func TestT03_Security_NonMemberCannotUpdate(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	n := len(e.auditList())
	var first string
	for _, id := range []string{p.ID, uuid.NewString(), "bukan-uuid"} {
		w := e.do(e.C, http.MethodPatch, "/projects/"+id, map[string]string{"name": "Hacked"})
		b := parseErr(w)
		if w.Code != 404 || b.Error.Code != "NOT_FOUND" || b.Error.Message != "project tidak ditemukan" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if first == "" {
			first = normBody(w)
		} else if normBody(w) != first {
			t.Fatalf("body tidak seragam: %s vs %s", normBody(w), first)
		}
	}
	if d := e.detail(e.A, p.ID); d.Name != "Alpha" {
		t.Fatalf("nama berubah: %s", d.Name)
	}
	if len(e.auditList()) != n {
		t.Fatal("audit tidak boleh bertambah")
	}
}

// ---- changememberrole ----

func TestChangeRole_RouteRegistered(t *testing.T) {
	e := setup(t)
	routePresent(t, e, "PATCH /api/projects/{projectID}/members/{userID}")
	routePresent(t, e, "DELETE /api/projects/{projectID}/members/{userID}")
}

func TestChangeRole_CSRFAndAuth(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	path := "/projects/" + p.ID + "/members/" + e.B
	w := e.rawDo(e.A, http.MethodPatch, path, `{"role":"owner"}`, false)
	mustCode(t, w, 403)
	if parseErr(w).Error.Code != "CSRF_REQUIRED" {
		t.Fatalf("%s", w.Body.String())
	}
	mustCode(t, e.do("", http.MethodPatch, path, map[string]string{"role": "owner"}), 401)
	if r := e.roleInDB(p.ID, e.B); r != "member" {
		t.Fatalf("role berubah: %s", r)
	}
}

func TestChangeRole_ConcurrentWithRemove(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.C, "owner"), 201)
		var w *httptest.ResponseRecorder
		both(
			func() {
				w = e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.C, map[string]string{"role": "member"})
			},
			func() { e.execSQL(`DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, p.ID, e.C) },
		)
		if w.Code == 404 {
			if m := parseErr(w).Error.Message; m != "anggota tidak ditemukan" {
				t.Fatalf("pesan 404: %s", m)
			}
		} else if w.Code != 200 {
			t.Fatalf("putaran %d: %d %s", i, w.Code, w.Body.String())
		}
		if e.ownerCount(p.ID) < 1 {
			t.Fatalf("putaran %d: owner habis", i)
		}
	}
	if n := e.orphanMembers(); n != 0 {
		t.Fatalf("baris yatim: %d", n)
	}
}

func TestChangeRole_RaceWithDeleteProject(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
		var code int
		both(
			func() {
				code = e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.B, map[string]string{"role": "owner"}).Code
			},
			func() { e.execSQL(`DELETE FROM projects WHERE id = $1`, p.ID) },
		)
		if code != 200 && code != 404 {
			t.Fatalf("putaran %d: %d", i, code)
		}
	}
	if n := e.orphanMembers(); n != 0 {
		t.Fatalf("baris yatim: %d", n)
	}
}

func TestChangeRole_AuditFailOpen(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	e.auditFail.Store(true)
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.B, map[string]string{"role": "owner"}), 200)
	if r := e.roleInDB(p.ID, e.B); r != "owner" {
		t.Fatalf("role %s", r)
	}

	e2 := setupWith(t, setupOpts{noAudit: true})
	p2 := e2.create(e2.A, "Alpha")
	mustCode(t, e2.addMember(e2.A, p2.ID, e2.B, "member"), 201)
	mustCode(t, e2.do(e2.A, http.MethodPatch, "/projects/"+p2.ID+"/members/"+e2.B, map[string]string{"role": "owner"}), 200)
	if r := e2.roleInDB(p2.ID, e2.B); r != "owner" {
		t.Fatalf("role %s", r)
	}
}

func TestChangeRole_DeactivatedTargetCanStillBeChanged(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	e.setActive(e.B, false)
	w := e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.B, map[string]string{"role": "owner"})
	mustCode(t, w, 200)
	var m base.MemberDTO
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.IsActive || m.Role != "owner" || !m.Found {
		t.Fatalf("%+v", m)
	}
	if r := e.roleInDB(p.ID, e.B); r != "owner" {
		t.Fatalf("role %s", r)
	}
}

// ---- removemember ----

func TestRemove_RouteRegistered(t *testing.T) {
	e := setup(t)
	routePresent(t, e, "DELETE /api/projects/{projectID}/members/{userID}")
	routePresent(t, e, "PATCH /api/projects/{projectID}/members/{userID}")
}

func TestRemove_CSRFAndAuth(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	path := "/projects/" + p.ID + "/members/" + e.B
	w := e.rawDo(e.A, http.MethodDelete, path, "", false)
	mustCode(t, w, 403)
	if parseErr(w).Error.Code != "CSRF_REQUIRED" {
		t.Fatalf("%s", w.Body.String())
	}
	mustCode(t, e.do("", http.MethodDelete, path, nil), 401)
	if e.count("project_members", p.ID) != 2 {
		t.Fatal("anggota berubah")
	}
}

func TestT03_AC3_ConcurrentMutualRemoval(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.C, "owner"), 201)
		before := len(e.auditActions(record.ActionProjectMemberRemoved))
		var c1, c2 int
		both(
			func() { c1 = e.do(e.A, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.C, nil).Code },
			func() { c2 = e.do(e.C, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.A, nil).Code },
		)
		ok := 0
		for _, c := range []int{c1, c2} {
			switch c {
			case 204:
				ok++
			case 404, 409, 403:
			default:
				t.Fatalf("putaran %d: status %d %d", i, c1, c2)
			}
		}
		if ok != 1 {
			t.Fatalf("putaran %d: harus tepat satu 204, got %d %d", i, c1, c2)
		}
		if e.count("project_members", p.ID) != 1 || e.ownerCount(p.ID) != 1 {
			t.Fatalf("putaran %d: anggota=%d owner=%d", i, e.count("project_members", p.ID), e.ownerCount(p.ID))
		}
		if got := len(e.auditActions(record.ActionProjectMemberRemoved)) - before; got != ok {
			t.Fatalf("putaran %d: audit %d != %d", i, got, ok)
		}
	}
}

func TestRemove_RaceWithDeleteProject(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
		var code int
		both(
			func() { code = e.do(e.A, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.B, nil).Code },
			func() { e.execSQL(`DELETE FROM projects WHERE id = $1`, p.ID) },
		)
		if code != 204 && code != 404 {
			t.Fatalf("putaran %d: %d", i, code)
		}
	}
	if n := e.orphanMembers(); n != 0 {
		t.Fatalf("baris yatim: %d", n)
	}
}

func TestRemove_AuditFailOpen(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	e.auditFail.Store(true)
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.B, nil), 204)
	if r := e.roleInDB(p.ID, e.B); r != "" {
		t.Fatalf("anggota masih ada: %s", r)
	}

	e2 := setupWith(t, setupOpts{noAudit: true})
	p2 := e2.create(e2.A, "Alpha")
	mustCode(t, e2.addMember(e2.A, p2.ID, e2.B, "member"), 201)
	mustCode(t, e2.do(e2.A, http.MethodDelete, "/projects/"+p2.ID+"/members/"+e2.B, nil), 204)
	if r := e2.roleInDB(p2.ID, e2.B); r != "" {
		t.Fatalf("anggota masih ada: %s", r)
	}
}

func TestRemove_ProjectSurvivesWithoutMembers(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.do(e.S, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.A, nil), 204)
	if !e.projectExists(p.ID) {
		t.Fatal("project ikut terhapus")
	}
	if e.count("project_members", p.ID) != 0 {
		t.Fatal("project_members tidak kosong")
	}
	if d := e.detail(e.S, p.ID); d.MemberCount != 0 || d.OwnerCount != 0 {
		t.Fatalf("%+v", d)
	}
}

// ---- addmember ----

func TestT03_AddMember_ConcurrentSameTarget(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.C, "owner"), 201)
		before := len(e.auditActions(record.ActionProjectMemberAdded))
		var w1, w2 *httptest.ResponseRecorder
		both(
			func() { w1 = e.addMember(e.A, p.ID, e.B, "member") },
			func() { w2 = e.addMember(e.C, p.ID, e.B, "member") },
		)
		created, conflict := 0, 0
		for _, w := range []*httptest.ResponseRecorder{w1, w2} {
			switch w.Code {
			case 201:
				created++
			case 409:
				conflict++
				if parseErr(w).Error.Details["reason"] != "ALREADY_MEMBER" {
					t.Fatalf("%s", w.Body.String())
				}
			default:
				t.Fatalf("putaran %d: %d %s", i, w.Code, w.Body.String())
			}
		}
		if created != 1 || conflict != 1 {
			t.Fatalf("putaran %d: 201=%d 409=%d", i, created, conflict)
		}
		var n int
		if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members WHERE project_id = $1 AND user_id = $2`, p.ID, e.B); err != nil || n != 1 {
			t.Fatalf("baris B: %d %v", n, err)
		}
		if got := len(e.auditActions(record.ActionProjectMemberAdded)) - before; got != 1 {
			t.Fatalf("putaran %d: audit %d", i, got)
		}
	}
}

func TestT03_AddMember_ActorDemotedRace(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.C, "owner"), 201)
		var demote, add *httptest.ResponseRecorder
		both(
			func() {
				demote = e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.C, map[string]string{"role": "member"})
			},
			func() { add = e.addMember(e.C, p.ID, e.B, "member") },
		)
		mustCode(t, demote, 200)
		switch add.Code {
		case 201:
			if e.roleInDB(p.ID, e.B) != "member" {
				t.Fatalf("putaran %d: B tidak tersimpan", i)
			}
		case 403:
			if e.roleInDB(p.ID, e.B) != "" {
				t.Fatalf("putaran %d: B tertambah padahal 403", i)
			}
		default:
			t.Fatalf("putaran %d: %d %s", i, add.Code, add.Body.String())
		}
		if e.roleInDB(p.ID, e.C) != "member" {
			t.Fatalf("putaran %d: C bukan member", i)
		}
	}
}

// ---- getproject / listprojects ----

func TestT03_GetProject_NoAudit(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	n := len(e.auditList())
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+p.ID, nil), 200)
	mustCode(t, e.do(e.C, http.MethodGet, "/projects/"+p.ID, nil), 404)
	mustCode(t, e.do(e.A, http.MethodGet, "/projects/"+uuid.NewString(), nil), 404)
	if len(e.auditList()) != n {
		t.Fatal("GET tidak boleh mengaudit")
	}
}

func TestT03_GetProject_NoOwnerProject(t *testing.T) {
	e := setup(t)
	w := e.do(e.S, http.MethodPost, "/projects", map[string]string{"name": "Kosong"})
	mustCode(t, w, 201)
	var sp base.ProjectDetail
	_ = json.Unmarshal(w.Body.Bytes(), &sp)
	d := e.detail(e.S, sp.ID)
	if d.MemberCount != 0 || d.OwnerCount != 0 || d.AccessLevel != "admin" {
		t.Fatalf("%+v", d)
	}
}

func TestT03_GetProject_Unauthenticated(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.do("", http.MethodGet, "/projects/"+p.ID, nil), 401)
}

func TestT03_ListProjects_NoAuditEntry(t *testing.T) {
	e := setup(t)
	e.create(e.A, "Alpha")
	n := len(e.auditList())
	for i := 0; i < 3; i++ {
		e.list(e.A)
		e.list(e.S)
	}
	mustCode(t, e.do(e.A, http.MethodGet, "/projects?limit=500", nil), 400)
	if len(e.auditList()) != n {
		t.Fatal("GET /projects tidak boleh mengaudit")
	}
}
