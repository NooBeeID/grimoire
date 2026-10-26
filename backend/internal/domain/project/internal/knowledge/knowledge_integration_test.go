//go:build integration

package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/buslistfiles"
	"grimoire/internal/domain/project/internal/knowledge/busreadfile"
	"grimoire/internal/domain/project/internal/knowledge/bussearch"
	"grimoire/internal/domain/project/internal/knowledge/busupsertfiles"
	"grimoire/internal/domain/project/internal/knowledge/countfiles"
	"grimoire/internal/domain/project/internal/knowledge/createfile"
	"grimoire/internal/domain/project/internal/knowledge/deletefile"
	"grimoire/internal/domain/project/internal/knowledge/deletefolder"
	"grimoire/internal/domain/project/internal/knowledge/getfile"
	"grimoire/internal/domain/project/internal/knowledge/listtree"
	"grimoire/internal/domain/project/internal/knowledge/movefile"
	"grimoire/internal/domain/project/internal/knowledge/search"
	"grimoire/internal/domain/project/internal/knowledge/updatefile"
	"grimoire/internal/domain/project/internal/knowledge/uploadfiles"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/knowledge/listfiles"
	"grimoire/lib/contracts/project/knowledge/readfile"
	ksearch "grimoire/lib/contracts/project/knowledge/search"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
	"grimoire/lib/contracts/project/management/checkaccess"
	"grimoire/lib/contracts/project/management/contentcount"
	"grimoire/lib/contracts/project/management/getproject"
)

// ---------------------------------------------------------------- harness

type env struct {
	t      *testing.T
	db     *db.DB
	bus    *bus.Registry
	router http.Handler

	mu       sync.Mutex
	members  map[string]map[string]bool // projectID -> userID -> member
	projects map[string]string          // projectID -> name
	audits   []record.Request
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// registerAll memanggil 13 Init persis seperti knowledge.Register setelah tahap wiring.
func registerAll(d app.Deps) {
	createfile.Init(d)
	getfile.Init(d)
	listtree.Init(d)
	countfiles.Init(d)
	uploadfiles.Init(d)
	updatefile.Init(d)
	deletefile.Init(d)
	deletefolder.Init(d)
	movefile.Init(d)
	search.Init(d)
	buslistfiles.Init(d)
	busreadfile.Init(d)
	bussearch.Init(d)
	busupsertfiles.Init(d)
}

func testConfig(maxProjectFiles int) config.Config {
	return config.Config{
		MaxFileBytes: 1 << 20, MaxUploadBytes: 50 << 20, MaxUploadFiles: 500,
		MaxProjectFiles: maxProjectFiles, MCPMaxResponseBytes: 102400,
	}
}

func newEnv(t *testing.T, maxProjectFiles int) *env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: d, members: map[string]map[string]bool{}, projects: map[string]string{}}
	e.bus = bus.NewRegistry(discardLog())

	e.bus.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
		r := req.(checkaccess.Request)
		e.mu.Lock()
		defer e.mu.Unlock()
		if _, ok := e.projects[r.ProjectID]; !ok {
			return checkaccess.Response{}, nil
		}
		lv := checkaccess.LevelNone
		if e.members[r.ProjectID][r.UserID] {
			lv = checkaccess.LevelMember
		}
		return checkaccess.Response{ProjectExists: true, Level: lv}, nil
	})
	e.bus.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		r := req.(getproject.Request)
		e.mu.Lock()
		defer e.mu.Unlock()
		n, ok := e.projects[r.ProjectID]
		return getproject.Response{Found: ok, ID: r.ProjectID, Name: n}, nil
	})
	e.bus.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.audits = append(e.audits, req.(record.Request))
		return record.Response{}, nil
	})

	api := chi.NewRouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := r.Header.Get("X-Test-User"); u != "" {
				r = r.WithContext(reqctx.WithIdentity(r.Context(),
					reqctx.Identity{UserID: u, Role: "developer", Name: "User-" + u[:4], Email: u[:4] + "@x.id"}))
			}
			next.ServeHTTP(w, r)
		})
	})
	root := chi.NewRouter()
	root.Mount("/api", api)
	e.router = root
	registerAll(app.Deps{
		Ctx: context.Background(), Config: testConfig(maxProjectFiles), Log: discardLog(),
		DB: d, Bus: e.bus, API: api, MCP: chi.NewRouter(),
	})
	return e
}

func (e *env) newProject(name string, members ...string) string {
	e.t.Helper()
	name = "Proj " + name // projects_name_len: 2-100 karakter
	var id string
	if err := e.db.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	e.mu.Lock()
	e.projects[id] = name
	e.members[id] = map[string]bool{}
	for _, m := range members {
		e.members[id][m] = true
	}
	e.mu.Unlock()
	return id
}

func newUser() string { return uuid.NewString() }

type resp struct {
	Code int
	Body []byte
}

func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("json: %v: %s", err, r.Body)
	}
}

func (r resp) errReason() string {
	var b struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.Body, &b)
	s, _ := b.Error.Details["reason"].(string)
	return s
}

func (e *env) do(user, method, path string, body io.Reader, ctype string) resp {
	rq := httptest.NewRequest(method, "/api"+path, body)
	if ctype != "" {
		rq.Header.Set("Content-Type", ctype)
	}
	if user != "" {
		rq.Header.Set("X-Test-User", user)
	}
	rq.Header.Set("User-Agent", "itest")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, rq)
	return resp{Code: w.Code, Body: w.Body.Bytes()}
}

func (e *env) doJSON(user, method, path string, v any) resp {
	b, _ := json.Marshal(v)
	return e.do(user, method, path, bytes.NewReader(b), "application/json")
}

func (e *env) get(user, path string) resp { return e.do(user, http.MethodGet, path, nil, "") }

type part struct{ name, data string }

func (e *env) upload(user, pid string, parts ...part) resp {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, p.name))
		h.Set("Content-Type", "text/markdown")
		w, _ := mw.CreatePart(h)
		_, _ = w.Write([]byte(p.data))
	}
	_ = mw.Close()
	return e.do(user, http.MethodPost, "/projects/"+pid+"/knowledge/upload", &buf, mw.FormDataContentType())
}

type uploadRes struct {
	Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int }
	Items   []struct{ Path, Status, Reason string }
}

func (e *env) mustUpload(user, pid string, parts ...part) uploadRes {
	e.t.Helper()
	r := e.upload(user, pid, parts...)
	if r.Code != 200 {
		e.t.Fatalf("upload %d %s", r.Code, r.Body)
	}
	var u uploadRes
	r.json(e.t, &u)
	return u
}

func (e *env) create(user, pid, path, content string) resp {
	return e.doJSON(user, http.MethodPost, "/projects/"+pid+"/knowledge/files", map[string]string{"path": path, "content": content})
}

type fileDTO struct {
	Path, Title, Content, Version string
}

func (e *env) getFile(user, pid, path string) (resp, fileDTO) {
	r := e.get(user, "/projects/"+pid+"/knowledge/files?path="+url.QueryEscape(path))
	var f fileDTO
	if r.Code == 200 {
		r.json(e.t, &f)
	}
	return r, f
}

func (e *env) update(user, pid, path, content, version string) resp {
	return e.doJSON(user, http.MethodPut, "/projects/"+pid+"/knowledge/files?path="+url.QueryEscape(path),
		map[string]string{"content": content, "version": version})
}

type treeRes struct {
	Prefix  string
	Folders []struct {
		Name, Path string
		FileCount  int `json:"file_count"`
	}
	Files []struct {
		Name, Path string
	}
	TotalFiles int `json:"total_files"`
}

func (e *env) tree(user, pid, prefix string) treeRes {
	e.t.Helper()
	r := e.get(user, "/projects/"+pid+"/knowledge/tree?prefix="+url.QueryEscape(prefix))
	if r.Code != 200 {
		e.t.Fatalf("tree %d %s", r.Code, r.Body)
	}
	var tr treeRes
	r.json(e.t, &tr)
	return tr
}

func (e *env) countRows(pid string) int {
	e.t.Helper()
	var n int
	if err := e.db.GetContext(context.Background(), &n, `SELECT count(*) FROM knowledge_files WHERE project_id=$1`, pid); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) auditCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.audits)
}

func (e *env) auditsSnapshot() []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]record.Request(nil), e.audits...)
}

func statusOf(u uploadRes, path string) (string, string) {
	for _, it := range u.Items {
		if it.Path == path {
			return it.Status, it.Reason
		}
	}
	return "", ""
}

func filePaths(tr treeRes) []string {
	var ps []string
	for _, f := range tr.Files {
		ps = append(ps, f.Path)
	}
	sort.Strings(ps)
	return ps
}

// ---------------------------------------------------------------- AC1

func TestAC1_UploadThreeLevels(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	if r := e.create(u, pid, "other.md", "# Lain"); r.Code != 201 {
		t.Fatalf("create %d %s", r.Code, r.Body)
	}
	files := []part{
		{"docs/a.md", "# A"},
		{"docs/auth/login.md", "# Login"},
		{"docs/auth/flow/x.md", "# X"},
	}
	r1 := e.mustUpload(u, pid, files...)
	if r1.Summary.Created != 3 || r1.Summary.Rejected != 0 {
		t.Fatalf("%+v", r1.Summary)
	}

	tr := e.tree(u, pid, "docs")
	if tr.TotalFiles != 3 || len(tr.Folders) != 1 || tr.Folders[0].Path != "docs/auth" || tr.Folders[0].FileCount != 2 ||
		len(tr.Files) != 1 || tr.Files[0].Path != "docs/a.md" {
		t.Fatalf("tree docs %+v", tr)
	}
	tr = e.tree(u, pid, "docs/auth")
	if tr.TotalFiles != 2 || len(tr.Folders) != 1 || tr.Folders[0].Path != "docs/auth/flow" ||
		len(tr.Files) != 1 || tr.Files[0].Path != "docs/auth/login.md" {
		t.Fatalf("tree docs/auth %+v", tr)
	}

	r2 := e.mustUpload(u, pid, files...)
	if r2.Summary.Unchanged != 3 || r2.Summary.Created != 0 || r2.Summary.Updated != 0 {
		t.Fatalf("reupload %+v", r2.Summary)
	}

	files[1].data = "# Login v2"
	r3 := e.mustUpload(u, pid, files...)
	if r3.Summary.Updated != 1 || r3.Summary.Unchanged != 2 {
		t.Fatalf("modified %+v", r3.Summary)
	}
	if st, _ := statusOf(r3, "docs/auth/login.md"); st != "updated" {
		t.Fatalf("status %s", st)
	}

	if rr, f := e.getFile(u, pid, "other.md"); rr.Code != 200 || f.Content != "# Lain" {
		t.Fatalf("other.md %d %s", rr.Code, rr.Body)
	}
	if got := e.tree(u, pid, "").TotalFiles; got != 4 {
		t.Fatalf("total %d", got)
	}
}

// ---------------------------------------------------------------- AC2

func TestAC2_UploadMixedRejections(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	u1 := e.mustUpload(u, pid,
		part{"../x.md", "x"},
		part{strings.Repeat("a", 600) + ".md", "x"},
		part{"big.md", strings.Repeat("a", 2<<20)},
		part{"bin.md", "ab\x00cd"},
		part{".git/config", "x"},
		part{".DS_Store", "x"},
		part{"ok/one.md", "# Satu"},
		part{"ok/two.txt", "dua"},
	)
	if u1.Summary.Rejected != 4 || u1.Summary.Skipped != 2 || u1.Summary.Created != 2 {
		t.Fatalf("%+v items=%+v", u1.Summary, u1.Items)
	}
	want := map[string]string{
		"../x.md": "path_traversal", "big.md": "too_large", "bin.md": "invalid_encoding",
	}
	for p, reason := range want {
		if st, rs := statusOf(u1, p); st != "rejected" || rs != reason {
			t.Fatalf("%s: %s/%s", p, st, rs)
		}
	}
	longPath := strings.Repeat("a", 600) + ".md"
	if st, rs := statusOf(u1, longPath); st != "rejected" || rs != "path_too_long" {
		t.Fatalf("long: %s/%s", st, rs)
	}
	if st, rs := statusOf(u1, ".git/config"); st != "skipped" || rs != "hidden" {
		t.Fatalf("git: %s/%s", st, rs)
	}
	if st, _ := statusOf(u1, ".DS_Store"); st != "skipped" {
		t.Fatalf("ds_store: %s", st)
	}
	for _, p := range []string{"ok/one.md", "ok/two.txt"} {
		if rr, _ := e.getFile(u, pid, p); rr.Code != 200 {
			t.Fatalf("%s %d", p, rr.Code)
		}
	}
	if n := e.countRows(pid); n != 2 {
		t.Fatalf("rows %d", n)
	}
}

// ---------------------------------------------------------------- AC3

func TestAC3_VersionConflict(t *testing.T) {
	e := newEnv(t, 5000)
	a, b := newUser(), newUser()
	pid := e.newProject("P", a, b)
	if r := e.create(a, pid, "doc.md", "# Awal"); r.Code != 201 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	_, fa := e.getFile(a, pid, "doc.md")
	_, fb := e.getFile(b, pid, "doc.md")
	if fa.Version == "" || fa.Version != fb.Version {
		t.Fatalf("versions %q %q", fa.Version, fb.Version)
	}

	if r := e.update(a, pid, "doc.md", "# Versi A", fa.Version); r.Code != 200 {
		t.Fatalf("A: %d %s", r.Code, r.Body)
	}
	r := e.update(b, pid, "doc.md", "# Versi B", fb.Version)
	if r.Code != 409 || r.errReason() != "version_conflict" {
		t.Fatalf("B: %d %s", r.Code, r.Body)
	}
	var body struct {
		Error struct {
			Details struct {
				Current struct{ Content string } `json:"current"`
			} `json:"details"`
		} `json:"error"`
	}
	r.json(t, &body)
	if body.Error.Details.Current.Content != "# Versi A" {
		t.Fatalf("current %q", body.Error.Details.Current.Content)
	}
	if _, f := e.getFile(a, pid, "doc.md"); f.Content != "# Versi A" {
		t.Fatalf("stored %q", f.Content)
	}
	// negatif: versi tidak valid
	if r := e.update(a, pid, "doc.md", "x", "bukan-hex"); r.Code != 400 || r.errReason() != "invalid_version" {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}

// ---------------------------------------------------------------- AC4

func TestAC4_SearchRankingAndHostileQueries(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	long := strings.Repeat("kalimat panjang tentang arsitektur sistem dan deployment. ", 400) + " login " +
		strings.Repeat("penutup dokumen panduan. ", 400)
	e.mustUpload(u, pid, part{"docs/login.md", "# Masuk\nhalaman singkat"}, part{"docs/guide.md", "# Panduan\n" + long})

	r := e.get(u, "/projects/"+pid+"/knowledge/search?q=login")
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	var sr struct {
		Items []struct{ Path string }
		Total int
	}
	r.json(t, &sr)
	if sr.Total != 2 || len(sr.Items) != 2 || sr.Items[0].Path != "docs/login.md" {
		t.Fatalf("%+v", sr)
	}

	for _, q := range []string{`"`, `&`, `:`, `or`, `-`, `!`, `"unclosed`, `a & | ! ( ) :*`, `'`, `\`} {
		rr := e.get(u, "/projects/"+pid+"/knowledge/search?q="+url.QueryEscape(q))
		if rr.Code != 200 {
			t.Fatalf("q=%q -> %d %s", q, rr.Code, rr.Body)
		}
	}
	if rr := e.get(u, "/projects/"+pid+"/knowledge/search?q=login&limit=abc"); rr.Code != 400 || rr.errReason() != "invalid_limit" {
		t.Fatalf("limit: %d %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------- AC6

func TestAC6_NonMemberGets404OnAllRoutes(t *testing.T) {
	e := newEnv(t, 5000)
	member, outsider := newUser(), newUser()
	pid := e.newProject("P", member)
	e.mustUpload(member, pid, part{"docs/a.md", "# A"})

	type call struct{ method, path, body, ctype string }
	calls := func(p string) []call {
		base := "/projects/" + p + "/knowledge"
		return []call{
			{http.MethodPost, base + "/files", `{"path":"n.md","content":"x"}`, "application/json"},
			{http.MethodGet, base + "/files?path=docs%2Fa.md", "", ""},
			{http.MethodPut, base + "/files?path=docs%2Fa.md", `{"content":"x","version":"` + strings.Repeat("a", 64) + `"}`, "application/json"},
			{http.MethodDelete, base + "/files?path=docs%2Fa.md", "", ""},
			{http.MethodDelete, base + "/folders?path=docs", "", ""},
			{http.MethodPost, base + "/files/move", `{"from":"docs/a.md","to":"docs/b.md"}`, "application/json"},
			{http.MethodGet, base + "/tree", "", ""},
			{http.MethodGet, base + "/search?q=a", "", ""},
		}
	}
	for _, p := range []string{pid, "bukan-uuid", uuid.NewString()} {
		for _, c := range calls(p) {
			r := e.do(outsider, c.method, c.path, strings.NewReader(c.body), c.ctype)
			if r.Code != 404 {
				t.Fatalf("%s %s -> %d %s", c.method, c.path, r.Code, r.Body)
			}
		}
		r := e.upload(outsider, p, part{"a.md", "x"})
		if r.Code != 404 {
			t.Fatalf("upload %s -> %d", p, r.Code)
		}
	}
	// negatif: tanpa sesi 401
	if r := e.get("", "/projects/"+pid+"/knowledge/tree"); r.Code != 401 {
		t.Fatalf("anon %d", r.Code)
	}
	if n := e.countRows(pid); n != 1 {
		t.Fatalf("data berubah: %d", n)
	}
}

func TestAC6_DeleteFolderAllDepthsButNotSiblings(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pa, pb := e.newProject("A", u), e.newProject("B", u)
	e.mustUpload(u, pa,
		part{"docs/a.md", "a"}, part{"docs/x/y/z.md", "z"}, part{"docs/x/b.md", "b"},
		part{"docs2/x.md", "x"}, part{"docs.md", "d"})
	e.mustUpload(u, pb, part{"docs/a.md", "a"})

	r := e.do(u, http.MethodDelete, "/projects/"+pa+"/knowledge/folders?path=docs", nil, "")
	var del struct{ Deleted int }
	r.json(t, &del)
	if r.Code != 200 || del.Deleted != 3 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	left := filePaths(e.tree(u, pa, ""))
	if strings.Join(left, ",") != "docs.md" || e.tree(u, pa, "docs2").TotalFiles != 1 {
		t.Fatalf("left %v", left)
	}
	if e.countRows(pb) != 1 {
		t.Fatalf("project B terpengaruh")
	}
	// negatif: root dan folder kosong
	if r := e.do(u, http.MethodDelete, "/projects/"+pa+"/knowledge/folders?path=", nil, ""); r.Code != 400 || r.errReason() != "prefix_required" {
		t.Fatalf("root %d %s", r.Code, r.Body)
	}
	if r := e.do(u, http.MethodDelete, "/projects/"+pa+"/knowledge/folders?path=docs", nil, ""); r.Code != 404 {
		t.Fatalf("empty %d %s", r.Code, r.Body)
	}
}

// ---------------------------------------------------------------- AC7 + audit

func auditSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		var out []string
		for _, x := range s {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

func TestAC7_UploadOneAuditEntryTruncatedPaths(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("Payment", u)
	var parts []part
	for i := 0; i < 100; i++ {
		parts = append(parts, part{fmt.Sprintf("bulk/f%03d.md", i), fmt.Sprintf("# F%d", i)})
	}
	res := e.mustUpload(u, pid, parts...)
	if res.Summary.Created != 100 {
		t.Fatalf("%+v", res.Summary)
	}
	as := e.auditsSnapshot()
	if len(as) != 1 {
		t.Fatalf("audit entries %d", len(as))
	}
	a := as[0]
	if a.Action != record.ActionKnowledgeUploadCompleted || a.ProjectID != pid || a.ProjectLabel != "Proj Payment" ||
		a.ActorID != u || a.Status != record.StatusOK {
		t.Fatalf("%+v", a)
	}
	if got := auditSlice(a.Result["paths"]); len(got) != 20 {
		t.Fatalf("paths %d", len(got))
	}
	if tr, _ := a.Result["paths_truncated"].(bool); !tr {
		t.Fatalf("paths_truncated %v", a.Result["paths_truncated"])
	}
}

func TestAudit_OneEntryPerMutationNoneOnFailureNoContent(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	const secret = "RAHASIA-UNIK-7f3a9c"
	steps := []struct {
		name   string
		do     func() resp
		action string
	}{
		{"create", func() resp { return e.create(u, pid, "m/a.md", "# A "+secret) }, record.ActionKnowledgeFileCreated},
		{"upload", func() resp { return e.upload(u, pid, part{"m/b.md", "# B " + secret}) }, record.ActionKnowledgeUploadCompleted},
		{"update", func() resp {
			_, f := e.getFile(u, pid, "m/a.md")
			return e.update(u, pid, "m/a.md", "# A2 "+secret, f.Version)
		}, record.ActionKnowledgeFileUpdated},
		{"move", func() resp {
			return e.doJSON(u, http.MethodPost, "/projects/"+pid+"/knowledge/files/move", map[string]string{"from": "m/a.md", "to": "m/c.md"})
		}, record.ActionKnowledgeFileMoved},
		{"delete-file", func() resp {
			return e.do(u, http.MethodDelete, "/projects/"+pid+"/knowledge/files?path=m%2Fc.md", nil, "")
		}, record.ActionKnowledgeFileDeleted},
		{"delete-folder", func() resp {
			return e.do(u, http.MethodDelete, "/projects/"+pid+"/knowledge/folders?path=m", nil, "")
		}, record.ActionKnowledgeFolderDeleted},
	}
	for i, s := range steps {
		before := e.auditCount()
		r := s.do()
		if r.Code >= 300 {
			t.Fatalf("%s: %d %s", s.name, r.Code, r.Body)
		}
		if got := e.auditCount() - before; got != 1 {
			t.Fatalf("%s: %d entri", s.name, got)
		}
		if last := e.auditsSnapshot()[before]; last.Action != s.action || last.ProjectID != pid {
			t.Fatalf("%s: %+v (step %d)", s.name, last, i)
		}
	}

	// gagal -> 0 entri
	e.create(u, pid, "z.md", "z")
	before := e.auditCount()
	failures := []resp{
		e.create(u, pid, "z.md", "z"), // path_exists
		e.update(u, pid, "z.md", "y", strings.Repeat("0", 64)),
		e.do(u, http.MethodDelete, "/projects/"+pid+"/knowledge/files?path=tidak-ada.md", nil, ""),
		e.doJSON(u, http.MethodPost, "/projects/"+pid+"/knowledge/files/move", map[string]string{"from": "tidak-ada.md", "to": "q.md"}),
		e.do(u, http.MethodDelete, "/projects/"+pid+"/knowledge/folders?path=kosong", nil, ""),
		e.create(u, pid, "../x.md", "x"),
	}
	for i, f := range failures {
		if f.Code < 400 {
			t.Fatalf("failure %d -> %d", i, f.Code)
		}
	}
	if got := e.auditCount() - before; got != 0 {
		t.Fatalf("gagal tetap mencatat %d entri", got)
	}

	for _, a := range e.auditsSnapshot() {
		if s := fmt.Sprintf("%v %v", a.Args, a.Result); strings.Contains(s, secret) {
			t.Fatalf("isi file bocor ke audit: %+v", a)
		}
	}
}

// ---------------------------------------------------------------- AC8

func TestAC8_ProjectIsolationHTTPAndBus(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pa, pb := e.newProject("A", u), e.newProject("B", u)
	e.mustUpload(u, pa, part{"same.md", "# Sama\nkataunikalpha"})
	e.mustUpload(u, pb, part{"same.md", "# Sama\nkataunikbeta"}, part{"only-b.md", "# B"})
	ctx := context.Background()

	// HTTP
	var sr struct {
		Items []struct{ Path string }
		Total int
	}
	e.get(u, "/projects/"+pa+"/knowledge/search?q=kataunikbeta").json(t, &sr)
	if sr.Total != 0 {
		t.Fatalf("search A memuat B: %+v", sr)
	}
	if _, f := e.getFile(u, pa, "same.md"); !strings.Contains(f.Content, "alpha") {
		t.Fatalf("%+v", f)
	}
	if r, _ := e.getFile(u, pa, "only-b.md"); r.Code != 404 {
		t.Fatalf("only-b via A: %d", r.Code)
	}

	// Bus
	lf, err := bus.CallTyped[listfiles.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
		listfiles.Request{ProjectID: pa, Recursive: true})
	if err != nil || lf.Total != 1 || lf.Items[0].Path != "same.md" {
		t.Fatalf("list A: %+v %v", lf, err)
	}
	rf, err := bus.CallTyped[readfile.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
		readfile.Request{ProjectID: pa, Path: "same.md"})
	if err != nil || !rf.Found || !strings.Contains(rf.Content, "alpha") || strings.Contains(rf.Content, "beta") {
		t.Fatalf("read A: %+v %v", rf, err)
	}
	rf, err = bus.CallTyped[readfile.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
		readfile.Request{ProjectID: pa, Path: "only-b.md"})
	if err != nil || rf.Found {
		t.Fatalf("read only-b via A: %+v %v", rf, err)
	}
	sb, err := bus.CallTyped[ksearch.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
		ksearch.Request{ProjectID: pa, Query: "kataunikbeta"})
	if err != nil || sb.Total != 0 || len(sb.Items) != 0 {
		t.Fatalf("bus search A: %+v %v", sb, err)
	}
	sb, err = bus.CallTyped[ksearch.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
		ksearch.Request{ProjectID: pb, Query: "kataunikbeta"})
	if err != nil || sb.Total != 1 || sb.Items[0].Path != "same.md" {
		t.Fatalf("bus search B: %+v %v", sb, err)
	}
}

func TestAC8_ReadFileMultibyteChaining(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	content := strings.Repeat("héllo wörld 日本語 😀 ", 40)
	if r := e.create(u, pid, "mb.md", content); r.Code != 201 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	var sb strings.Builder
	off, loops := 0, 0
	for {
		rf, err := bus.CallTyped[readfile.Response](context.Background(), e.bus, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
			readfile.Request{ProjectID: pid, Path: "/mb.md", Offset: off, Limit: 7})
		if err != nil || !rf.Found {
			t.Fatalf("%+v %v", rf, err)
		}
		if !utf8.ValidString(rf.Content) {
			t.Fatalf("potongan memotong rune: %q", rf.Content)
		}
		sb.WriteString(rf.Content)
		if !rf.Truncated {
			break
		}
		if rf.NextOffset <= off {
			t.Fatalf("offset tidak maju")
		}
		off = rf.NextOffset
		if loops++; loops > 5000 {
			t.Fatal("loop")
		}
	}
	if sb.String() != content {
		t.Fatalf("isi tidak utuh")
	}
}

// ---------------------------------------------------------------- kuota, count, move, cascade

func TestQuota_ParallelUploadsRespectLimit(t *testing.T) {
	e := newEnv(t, 3)
	u := newUser()
	pid := e.newProject("P", u)
	var wg sync.WaitGroup
	results := make([]uploadRes, 2)
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := e.upload(u, pid, part{fmt.Sprintf("p%d/a.md", i), "a"}, part{fmt.Sprintf("p%d/b.md", i), "b"})
			codes[i] = r.Code
			if r.Code == 200 {
				r.json(t, &results[i])
			}
		}(i)
	}
	wg.Wait()
	if codes[0] != 200 || codes[1] != 200 {
		t.Fatalf("codes %v", codes)
	}
	created, limited := 0, 0
	for _, r := range results {
		created += r.Summary.Created
		for _, it := range r.Items {
			if it.Reason == "project_file_limit" {
				limited++
			}
		}
	}
	if created != 3 || limited != 1 || e.countRows(pid) != 3 {
		t.Fatalf("created=%d limited=%d rows=%d", created, limited, e.countRows(pid))
	}
	// negatif: create tunggal juga ditolak saat penuh
	if r := e.create(u, pid, "extra.md", "x"); r.Code != 400 || r.errReason() != "project_file_limit" {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}

func TestCountFilesBus(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pa, pb := e.newProject("A", u), e.newProject("B", u)
	e.mustUpload(u, pa, part{"a.md", "a"}, part{"d/b.md", "b"})
	res, err := bus.CallTyped[contentcount.Response](context.Background(), e.bus, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
		contentcount.Request{ProjectIDs: []string{pa, pb, "bukan-uuid"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts[pa] != 2 || res.Counts[pb] != 0 || res.Counts["bukan-uuid"] != 0 {
		t.Fatalf("%v", res.Counts)
	}
}

func TestMoveKeepsSearchInSync(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("P", u)
	e.create(u, pid, "notes/todo.md", "# Todo\nisi biasa")
	search := func(q string) int {
		var sr struct{ Total int }
		e.get(u, "/projects/"+pid+"/knowledge/search?q="+q).json(t, &sr)
		return sr.Total
	}
	if search("zebrafish") != 0 {
		t.Fatal("seharusnya belum ada")
	}
	r := e.doJSON(u, http.MethodPost, "/projects/"+pid+"/knowledge/files/move", map[string]string{"from": "notes/todo.md", "to": "archive/zebrafish.md"})
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if search("zebrafish") != 1 {
		t.Fatal("tsvector tidak mengikuti path baru")
	}
	if rr, _ := e.getFile(u, pid, "notes/todo.md"); rr.Code != 404 {
		t.Fatalf("path lama %d", rr.Code)
	}
	// negatif: tujuan sudah ada, same_path
	e.create(u, pid, "archive/lain.md", "x")
	if r := e.doJSON(u, http.MethodPost, "/projects/"+pid+"/knowledge/files/move", map[string]string{"from": "archive/lain.md", "to": "archive/zebrafish.md"}); r.Code != 409 || r.errReason() != "path_exists" {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if r := e.doJSON(u, http.MethodPost, "/projects/"+pid+"/knowledge/files/move", map[string]string{"from": "archive/lain.md", "to": "archive/lain.md"}); r.Code != 400 || r.errReason() != "same_path" {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}

func TestCascadeOnProjectDelete(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pa, pb := e.newProject("A", u), e.newProject("B", u)
	e.mustUpload(u, pa, part{"a.md", "a"}, part{"d/b.md", "b"})
	e.mustUpload(u, pb, part{"a.md", "a"})
	if _, err := e.db.ExecContext(context.Background(), `DELETE FROM projects WHERE id=$1`, pa); err != nil {
		t.Fatal(err)
	}
	if e.countRows(pa) != 0 || e.countRows(pb) != 1 {
		t.Fatalf("A=%d B=%d", e.countRows(pa), e.countRows(pb))
	}
}

// ---------------------------------------------------------------- wiring

func TestRoutesRegistered(t *testing.T) {
	api := chi.NewRouter()
	b := bus.NewRegistry(discardLog())
	Register(app.Deps{
		Ctx: context.Background(), Config: testConfig(5000), Log: discardLog(),
		Bus: b, API: api, MCP: chi.NewRouter(),
	})
	got := map[string]bool{}
	err := chi.Walk(api, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const p = "/projects/{projectID}/knowledge"
	want := []string{
		"POST " + p + "/upload",
		"POST " + p + "/files",
		"GET " + p + "/files",
		"PUT " + p + "/files",
		"DELETE " + p + "/files",
		"DELETE " + p + "/folders",
		"POST " + p + "/files/move",
		"GET " + p + "/tree",
		"GET " + p + "/search",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("route tidak terdaftar: %s (ada: %v)", w, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("jumlah route %d, mau %d: %v", len(got), len(want), got)
	}
	for _, ep := range []string{
		constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
		constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
	} {
		// Request bertipe salah: handler terdaftar menolak dengan ErrBadRequestType, bukan ErrNotRegistered.
		_, err := b.Call(context.Background(), ep, struct{}{})
		if err == nil || errors.Is(err, bus.ErrNotRegistered) {
			t.Errorf("endpoint bus %s tidak terdaftar (err=%v)", ep, err)
		}
	}
}

// ---------------------------------------------------------------- AC9: EXPLAIN + p95

func TestAC9_ExplainAndP95(t *testing.T) {
	if testing.Short() {
		t.Skip("seed 5000 file; lewati pada -short")
	}
	e := newEnv(t, 100000)
	u := newUser()
	pid := e.newProject("Besar", u)
	other := e.newProject("Lain", u)
	ctx := context.Background()

	// 5000 file, ukuran 1-50 KB, isi campuran.
	_, err := e.db.ExecContext(ctx, `
INSERT INTO knowledge_files (project_id, path, title, content, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
SELECT $1::uuid, 'dir' || lpad((i % 50)::text, 2, '0') || '/file' || lpad(i::text, 4, '0') || '.md', 'Judul ' || i, c,
       sha256(convert_to(c, 'UTF8')), octet_length(c), $2::uuid, 'Seed', $2::uuid, 'Seed'
FROM (
  SELECT i, (SELECT string_agg('w' || (abs(hashtext(i::text || ':' || j::text)) % 3000), ' ') FROM generate_series(1, 200 * (1 + i % 50)) j)
         || ' modul' || (i % 97) || ' alur' || (i % 53) || ' login auth token sesi'
         || CASE WHEN i % 1000 = 0 THEN ' uniqterm' ELSE '' END AS c
  FROM generate_series(1, 5000) AS i
) s`, pid, u)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		e.create(u, other, fmt.Sprintf("o%02d.md", i), "# kecil")
	}
	if _, err := e.db.ExecContext(ctx, `ANALYZE knowledge_files`); err != nil {
		t.Fatal(err)
	}
	if n := e.countRows(pid); n != 5000 {
		t.Fatalf("seed %d", n)
	}

	explain := func(q string, args ...any) string {
		var lines []string
		if err := e.db.SelectContext(ctx, &lines, "EXPLAIN "+q, args...); err != nil {
			t.Fatal(err)
		}
		return strings.Join(lines, "\n")
	}
	pl := explain(`SELECT path, size_bytes, updated_at FROM knowledge_files WHERE project_id=$1 AND path LIKE 'dir07/%' ORDER BY path LIMIT 100`, pid)
	t.Logf("EXPLAIN ListMeta:\n%s", pl)
	if !strings.Contains(pl, "knowledge_files_project_path_prefix") {
		t.Errorf("ListMeta tidak memakai knowledge_files_project_path_prefix:\n%s", pl)
	}
	pl = explain(`SELECT path, ts_rank_cd(search, websearch_to_tsquery('simple', 'uniqterm'), 1) AS score FROM knowledge_files WHERE project_id=$1 AND search @@ websearch_to_tsquery('simple', 'uniqterm') AND path LIKE '%' ORDER BY score DESC LIMIT 20`, pid)
	t.Logf("EXPLAIN Search:\n%s", pl)
	if !strings.Contains(pl, "knowledge_files_search_gin") {
		t.Errorf("Search tidak memakai knowledge_files_search_gin:\n%s", pl)
	}

	// 50 kueri campuran.
	type op func() (int, time.Duration)
	timed := func(f func() int) (int, time.Duration) {
		s := time.Now()
		code := f()
		return code, time.Since(s)
	}
	var ops []op
	// "login" cocok ke semua 5000 dokumen (kasus terburuk); sisanya selektif.
	terms := []string{"login", "uniqterm", "modul42", "alur7", "modul5 alur5", "w1234", "dir07", "file0123"}
	for i := 0; i < 50; i++ {
		i := i
		switch i % 5 {
		case 0, 1:
			ops = append(ops, func() (int, time.Duration) {
				return timed(func() int {
					return e.get(u, "/projects/"+pid+"/knowledge/search?q="+url.QueryEscape(terms[i%len(terms)])).Code
				})
			})
		case 2:
			ops = append(ops, func() (int, time.Duration) {
				return timed(func() int {
					return e.get(u, "/projects/"+pid+"/knowledge/tree?prefix="+fmt.Sprintf("dir%02d", i%50)).Code
				})
			})
		case 3:
			ops = append(ops, func() (int, time.Duration) {
				return timed(func() int {
					_, err := bus.CallTyped[listfiles.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
						listfiles.Request{ProjectID: pid, Prefix: fmt.Sprintf("dir%02d", i%50), Recursive: true})
					if err != nil {
						return 500
					}
					return 200
				})
			})
		default:
			ops = append(ops, func() (int, time.Duration) {
				return timed(func() int {
					rf, err := bus.CallTyped[readfile.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
						readfile.Request{ProjectID: pid, Path: fmt.Sprintf("dir%02d/file%04d.md", i%50, 50+i%50)})
					if err != nil || !rf.Found {
						return 500
					}
					return 200
				})
			})
		}
	}
	var ds []time.Duration
	kinds := []string{"search", "search", "tree", "bus-list", "bus-read"}
	slowest := map[string]time.Duration{}
	for i, o := range ops {
		code, d := o()
		if code != 200 {
			t.Fatalf("kueri gagal: %d", code)
		}
		ds = append(ds, d)
		if k := kinds[i%5]; d > slowest[k] {
			slowest[k] = d
		}
	}
	t.Logf("terlambat per jenis: %v", slowest)
	sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
	p95 := ds[(len(ds)*95+99)/100-1]
	t.Logf("p95=%s max=%s (n=%d)", p95, ds[len(ds)-1], len(ds))
	if p95 > 200*time.Millisecond {
		t.Errorf("p95 %s > 200ms", p95)
	}
}

// Task 10 (T20): setiap writer mengisi kolom sections (tidak NULL, offset rune).
func TestWritersFillSections(t *testing.T) {
	e := newEnv(t, 5000)
	u := newUser()
	pid := e.newProject("Sec", u)
	ctx := context.Background()
	const doc = "😀 pra\n# Judul\nisi\n## Sub\nlagi"

	sectionsOf := func(path string) (n int, first string) {
		t.Helper()
		var raw []byte
		if err := e.db.GetContext(ctx, &raw, `SELECT sections FROM knowledge_files WHERE project_id=$1 AND path=$2`, pid, path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if raw == nil {
			t.Fatalf("%s: sections NULL", path)
		}
		var secs []struct {
			H string `json:"h"`
			S int    `json:"s"`
		}
		if err := json.Unmarshal(raw, &secs); err != nil {
			t.Fatal(err)
		}
		if len(secs) == 0 {
			return 0, ""
		}
		return len(secs), secs[len(secs)-1].H
	}

	// create
	if r := e.create(u, pid, "c.md", doc); r.Code != 201 {
		t.Fatalf("create %d %s", r.Code, r.Body)
	}
	if n, last := sectionsOf("c.md"); n != 3 || last != "Sub" {
		t.Fatalf("create sections n=%d last=%q", n, last)
	}

	// update: sections ikut berubah
	_, f := e.getFile(u, pid, "c.md")
	if r := e.update(u, pid, "c.md", "# Baru\nsatu", f.Version); r.Code != 200 {
		t.Fatalf("update %d %s", r.Code, r.Body)
	}
	if n, last := sectionsOf("c.md"); n != 1 || last != "Baru" {
		t.Fatalf("update sections n=%d last=%q", n, last)
	}

	// upload (create dan update)
	e.mustUpload(u, pid, part{"up.md", doc})
	if n, _ := sectionsOf("up.md"); n != 3 {
		t.Fatalf("upload create sections n=%d", n)
	}
	e.mustUpload(u, pid, part{"up.md", "# Ganti\nx"})
	if n, last := sectionsOf("up.md"); n != 1 || last != "Ganti" {
		t.Fatalf("upload update sections n=%d last=%q", n, last)
	}

	// upsert-files (bus git sync): created lalu updated
	upsert := func(content string) {
		t.Helper()
		res, err := bus.CallTyped[upsertfiles.Response](ctx, e.bus, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, upsertfiles.Request{
			ProjectID: pid, Actor: upsertfiles.Actor{UserID: u, Role: "developer", Label: "U"},
			Source: upsertfiles.SourceGitSync,
			Items:  []upsertfiles.Item{{Index: 0, Path: "g.md", Content: []byte(content)}},
		})
		if err != nil || len(res.Items) != 1 || (res.Items[0].Status != "created" && res.Items[0].Status != "updated") {
			t.Fatalf("upsert %+v %v", res, err)
		}
	}
	upsert(doc)
	if n, _ := sectionsOf("g.md"); n != 3 {
		t.Fatalf("upsert create sections n=%d", n)
	}
	upsert("# Git\nbaru")
	if n, last := sectionsOf("g.md"); n != 1 || last != "Git" {
		t.Fatalf("upsert update sections n=%d last=%q", n, last)
	}
}
