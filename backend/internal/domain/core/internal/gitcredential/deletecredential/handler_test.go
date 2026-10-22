package deletecredential

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/lib/common/reqctx"
)

// memRepo adalah repo in-memory yang memfilter per user.
type memRepo struct {
	mu   sync.Mutex
	rows map[delCall]bool
	got  []delCall
}

func (m *memRepo) Delete(_ context.Context, u string, p base.Provider, h string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := delCall{u, p, h}
	m.got = append(m.got, k)
	if m.rows[k] {
		delete(m.rows, k)
		return true, nil
	}
	return false, nil
}

func newTestRouter(repo repository, user string, a *fakeAudit) http.Handler {
	r := chi.NewRouter()
	if user != "" {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: user})))
			})
		})
	}
	r.Delete("/me/git-credentials/{provider}", newHandler(NewUsecase(repo, a)).ServeHTTP)
	return r
}

func doDelete(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func errCodeOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("bukan JSON: %s", w.Body.String())
	}
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

var traceRe = regexp.MustCompile(`,?"trace_id":"[^"]*"`)

func TestHandler_Delete(t *testing.T) {
	repo := &memRepo{rows: map[delCall]bool{
		{"A", base.ProviderGitHub, "github.com"}:    true,
		{"A", base.ProviderGitLab, "git.noobee.id"}: true,
		{"B", base.ProviderGitHub, "github.com"}:    true,
	}}
	a := &fakeAudit{}
	h := newTestRouter(repo, "A", a)

	w := doDelete(h, "/me/git-credentials/github", "")
	if w.Code != 204 || w.Body.Len() != 0 || strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatalf("own: %d %q %v", w.Code, w.Body.String(), w.Header())
	}

	w = doDelete(h, "/me/git-credentials/gitlab?host=git.noobee.id", "")
	if w.Code != 204 {
		t.Fatalf("gitlab: %d", w.Code)
	}
	if last := repo.got[len(repo.got)-1]; last.host != "git.noobee.id" {
		t.Fatalf("host %s", last.host)
	}

	// Ulang: 404 (A sudah tidak punya github); baris B tetap.
	w = doDelete(h, "/me/git-credentials/github", "")
	if w.Code != 404 || errCodeOf(t, w) != "NOT_FOUND" || !strings.Contains(w.Body.String(), "token tidak ditemukan") {
		t.Fatalf("ulang: %d %s", w.Code, w.Body.String())
	}
	if !repo.rows[delCall{"B", base.ProviderGitHub, "github.com"}] {
		t.Fatal("baris B terhapus")
	}
	if len(a.events) != 2 {
		t.Fatalf("audit %d", len(a.events))
	}
}

func TestHandler_OtherUserIdenticalTo404(t *testing.T) {
	repo := &memRepo{rows: map[delCall]bool{{"B", base.ProviderGitHub, "github.com"}: true}}
	w1 := doDelete(newTestRouter(repo, "A", &fakeAudit{}), "/me/git-credentials/github", "")
	w2 := doDelete(newTestRouter(&memRepo{rows: map[delCall]bool{}}, "A", &fakeAudit{}), "/me/git-credentials/github", "")
	if w1.Code != 404 || w2.Code != 404 {
		t.Fatalf("%d %d", w1.Code, w2.Code)
	}
	b1 := traceRe.ReplaceAllString(w1.Body.String(), "")
	b2 := traceRe.ReplaceAllString(w2.Body.String(), "")
	if b1 != b2 {
		t.Fatalf("beda:\n%s\n%s", b1, b2)
	}
	if !repo.rows[delCall{"B", base.ProviderGitHub, "github.com"}] {
		t.Fatal("baris B terhapus")
	}
}

func TestHandler_Errors(t *testing.T) {
	repo := &memRepo{rows: map[delCall]bool{}}
	cases := []struct {
		path string
		code int
		name string
	}{
		{"/me/git-credentials/bitbucket", 400, "INVALID_ARGUMENT"},
		{"/me/git-credentials/github?host=evil.com", 422, "HOST_NOT_ALLOWED"},
		{"/me/git-credentials/gitlab?host=user@host", 400, "INVALID_ARGUMENT"},
	}
	for _, c := range cases {
		w := doDelete(newTestRouter(repo, "A", &fakeAudit{}), c.path, "")
		if w.Code != c.code || errCodeOf(t, w) != c.name {
			t.Errorf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
	}
	if len(repo.got) != 0 {
		t.Fatal("repo tidak boleh dipanggil")
	}
}

func TestHandler_NoIdentity401(t *testing.T) {
	w := doDelete(newTestRouter(&memRepo{}, "", &fakeAudit{}), "/me/git-credentials/github", "")
	if w.Code != 401 || errCodeOf(t, w) != "UNAUTHORIZED" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestHandler_BodyIgnored(t *testing.T) {
	repo := &memRepo{rows: map[delCall]bool{{"A", base.ProviderGitHub, "github.com"}: true}}
	w := doDelete(newTestRouter(repo, "A", &fakeAudit{}), "/me/git-credentials/github", `{"x":1}`)
	if w.Code != 204 {
		t.Fatalf("%d", w.Code)
	}
}
