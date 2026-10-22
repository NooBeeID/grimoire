package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/secret"
)

const canary = "ghp_CANARY0123456789abcdefCANARY"

var tok = secret.New(canary)

type rec struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (r *rec) add(q *http.Request) { r.mu.Lock(); r.reqs = append(r.reqs, q); r.mu.Unlock() }

func newClient(t *testing.T, h http.HandlerFunc, gitlabAllowed bool) (*Client, *httptest.Server, *rec, string) {
	t.Helper()
	r := &rec{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.add(q)
		h(w, q)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	hc := safehttp.New(safehttp.Options{Timeout: 3 * time.Second, InsecureAllowHosts: []string{u.Host}})
	pol := HostPolicy{}
	if gitlabAllowed {
		pol.AllowedGitLabHosts = []string{u.Host}
	}
	return New(hc, pol, Options{GitHubAPIBase: srv.URL, InsecureHosts: []string{u.Host}}), srv, r, u.Host
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		hdr    map[string]string
		want   error
	}{
		{"401", 401, nil, ErrUnauthorized},
		{"403 rate", 403, map[string]string{"X-RateLimit-Remaining": "0"}, ErrRateLimited},
		{"403 retry-after", 403, map[string]string{"Retry-After": "7"}, ErrRateLimited},
		{"403 plain", 403, nil, ErrForbidden},
		{"404", 404, nil, ErrNotFound},
		{"429", 429, map[string]string{"Retry-After": "60"}, ErrRateLimited},
		{"500", 500, nil, ErrUpstream},
		{"502", 502, nil, ErrUpstream},
	}
	for _, p := range []Provider{GitHub, GitLab} {
		for _, tc := range cases {
			t.Run(string(p)+"/"+tc.name, func(t *testing.T) {
				c, _, _, host := newClient(t, func(w http.ResponseWriter, q *http.Request) {
					for k, v := range tc.hdr {
						w.Header().Set(k, v)
					}
					w.WriteHeader(tc.status)
					fmt.Fprint(w, "body mengandung rahasia "+canary)
				}, true)
				h := "github.com"
				if p == GitLab {
					h = host
				}
				err := c.VerifyToken(context.Background(), p, h, tok)
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v want %v", err, tc.want)
				}
				if strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "rahasia") {
					t.Fatalf("pesan error bocor: %v", err)
				}
				if tc.want == ErrRateLimited {
					var rl *RateLimitedError
					if !errors.As(err, &rl) {
						t.Fatal("RateLimitedError")
					}
					if tc.hdr["Retry-After"] == "60" && rl.RetryAfter != 60*time.Second {
						t.Fatal(rl.RetryAfter)
					}
				}
			})
		}
	}
}

func TestBadJSONIsUpstream(t *testing.T) {
	c, _, _, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, "{not json") }, false)
	_, err := c.ListTree(context.Background(), Repo{Provider: GitHub, Host: "github.com", Path: "o/r", Branch: "main"}, "", tok)
	if !errors.Is(err, ErrUpstream) {
		t.Fatal(err)
	}
}

func TestVerifyTokenOK(t *testing.T) {
	c, _, r, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, `{"login":"x"}`) }, false)
	if err := c.VerifyToken(context.Background(), GitHub, "github.com", tok); err != nil {
		t.Fatal(err)
	}
	q := r.reqs[0]
	if q.URL.Path != "/user" || q.Header.Get("Authorization") != "Bearer "+canary ||
		q.Header.Get("X-GitHub-Api-Version") != "2022-11-28" || q.Header.Get("User-Agent") != "grimoire-git-sync" {
		t.Fatalf("headers: %v %v", q.URL, q.Header)
	}
}

func TestGitHubTree(t *testing.T) {
	c, _, r, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) {
		fmt.Fprint(w, `{"truncated":false,"tree":[
		 {"path":"docs","type":"tree","sha":"t"},
		 {"path":"docs/a.md","type":"blob","sha":"s1","size":10},
		 {"path":"docs/sub/b.txt","type":"blob","sha":"s2","size":20},
		 {"path":"docsx/c.md","type":"blob","sha":"s3","size":1},
		 {"path":"README.md","type":"blob","sha":"s4"}]}`)
	}, false)
	repo := Repo{Provider: GitHub, Host: "github.com", Path: "noobee/grimoire-docs", Branch: "feature/x"}
	got, err := c.ListTree(context.Background(), repo, "docs", tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "docs/a.md" || got[0].Size != 10 || got[1].BlobSHA != "s2" {
		t.Fatalf("%+v", got)
	}
	all, _ := c.ListTree(context.Background(), repo, "", tok)
	if len(all) != 4 || all[3].Size != -1 {
		t.Fatalf("%+v", all)
	}
	q := r.reqs[0]
	if q.URL.EscapedPath() != "/repos/noobee/grimoire-docs/git/trees/feature%2Fx" || q.URL.RawQuery != "recursive=1" {
		t.Fatalf("url: %s ? %s", q.URL.EscapedPath(), q.URL.RawQuery)
	}
	for _, q := range r.reqs {
		if strings.Contains(q.URL.String(), canary) {
			t.Fatal("token di URL")
		}
	}
}

func TestGitHubTreeTruncated(t *testing.T) {
	c, _, _, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, `{"truncated":true,"tree":[]}`) }, false)
	_, err := c.ListTree(context.Background(), Repo{Provider: GitHub, Host: "github.com", Path: "o/r", Branch: "main"}, "", tok)
	if !errors.Is(err, ErrTreeTooLarge) {
		t.Fatal(err)
	}
}

func TestGitHubBlob(t *testing.T) {
	c, _, r, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, strings.Repeat("a", 10)) }, false)
	repo := Repo{Provider: GitHub, Host: "github.com", Path: "o/r", Branch: "main"}
	b, tl, err := c.FetchBlob(context.Background(), repo, "abc", 10, tok)
	if err != nil || tl || len(b) != 10 {
		t.Fatal(len(b), tl, err)
	}
	b, tl, err = c.FetchBlob(context.Background(), repo, "abc", 9, tok)
	if err != nil || !tl || b != nil {
		t.Fatal(len(b), tl, err)
	}
	if r.reqs[0].Header.Get("Accept") != "application/vnd.github.raw" || r.reqs[0].URL.Path != "/repos/o/r/git/blobs/abc" {
		t.Fatal(r.reqs[0].URL, r.reqs[0].Header)
	}
}

func TestGitLabTreeKeysetPagination(t *testing.T) {
	var host string
	c, _, r, h := newClient(t, func(w http.ResponseWriter, q *http.Request) {
		page := q.URL.Query().Get("page_token")
		switch page {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/api/v4/projects/grp%%2Fsub%%2Frepo/repository/tree?page_token=2>; rel="next"`, host))
			fmt.Fprint(w, `[{"id":"1","type":"blob","path":"api/a.yaml"},{"id":"x","type":"tree","path":"api/d"}]`)
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/api/v4/projects/grp%%2Fsub%%2Frepo/repository/tree?page_token=3>; rel="next"`, host))
			fmt.Fprint(w, `[{"id":"2","type":"blob","path":"api/b.yaml"}]`)
		default:
			fmt.Fprint(w, `[{"id":"3","type":"blob","path":"api/c.json"},{"id":"4","type":"blob","path":"other/z.md"}]`)
		}
	}, true)
	host = h
	got, err := c.ListTree(context.Background(), Repo{Provider: GitLab, Host: h, Path: "grp/sub/repo", Branch: "main"}, "api", tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].BlobSHA != "3" || got[0].Size != -1 {
		t.Fatalf("%+v", got)
	}
	if len(r.reqs) != 3 {
		t.Fatal(len(r.reqs))
	}
	first := r.reqs[0]
	if first.URL.EscapedPath() != "/api/v4/projects/grp%2Fsub%2Frepo/repository/tree" ||
		first.URL.Query().Get("ref") != "main" || first.URL.Query().Get("path") != "api" ||
		first.URL.Query().Get("pagination") != "keyset" || first.Header.Get("PRIVATE-TOKEN") != canary {
		t.Fatalf("%s %v", first.URL, first.Header)
	}
	for _, q := range r.reqs {
		if strings.Contains(q.URL.String(), canary) {
			t.Fatal("token di URL")
		}
	}
}

func TestGitLabLinkToOtherHostIgnored(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { t.Error("host lain tidak boleh dihubungi") }))
	defer other.Close()
	c, _, _, h := newClient(t, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/steal>; rel="next"`, other.URL))
		fmt.Fprint(w, `[{"id":"1","type":"blob","path":"a.md"}]`)
	}, true)
	got, err := c.ListTree(context.Background(), Repo{Provider: GitLab, Host: h, Path: "g/r", Branch: "main"}, "", tok)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
}

func TestGitLab51PagesTooLarge(t *testing.T) {
	var host string
	c, _, _, h := newClient(t, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/api/v4/projects/g%%2Fr/repository/tree?page_token=n>; rel="next"`, host))
		fmt.Fprint(w, `[]`)
	}, true)
	host = h
	_, err := c.ListTree(context.Background(), Repo{Provider: GitLab, Host: h, Path: "g/r", Branch: "main"}, "", tok)
	if !errors.Is(err, ErrTreeTooLarge) {
		t.Fatal(err)
	}
}

func TestGitLabBlob(t *testing.T) {
	c, _, r, h := newClient(t, func(w http.ResponseWriter, q *http.Request) { fmt.Fprint(w, "hello") }, true)
	b, tl, err := c.FetchBlob(context.Background(), Repo{Provider: GitLab, Host: h, Path: "g/r", Branch: "main"}, "sha1", 100, tok)
	if err != nil || tl || string(b) != "hello" {
		t.Fatal(string(b), tl, err)
	}
	if r.reqs[0].URL.EscapedPath() != "/api/v4/projects/g%2Fr/repository/blobs/sha1/raw" {
		t.Fatal(r.reqs[0].URL)
	}
}

func TestHostNotAllowedNoRequest(t *testing.T) {
	c, _, r, h := newClient(t, func(w http.ResponseWriter, q *http.Request) {}, false)
	ctx := context.Background()
	if err := c.VerifyToken(ctx, GitLab, h, tok); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatal(err)
	}
	if _, err := c.ListTree(ctx, Repo{Provider: GitHub, Host: "evil.com", Path: "o/r", Branch: "m"}, "", tok); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatal(err)
	}
	if _, _, err := c.FetchBlob(ctx, Repo{Provider: GitLab, Host: "evil.com", Path: "o/r"}, "s", 1, tok); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatal(err)
	}
	if len(r.reqs) != 0 {
		t.Fatal("tidak boleh ada request")
	}
}

func TestContextCancelPassesThrough(t *testing.T) {
	c, _, _, _ := newClient(t, func(w http.ResponseWriter, q *http.Request) { time.Sleep(200 * time.Millisecond) }, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := c.VerifyToken(ctx, GitHub, "github.com", tok)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestTransportErrorHidesToken(t *testing.T) {
	hc := safehttp.New(safehttp.Options{Timeout: time.Second})
	c := New(hc, HostPolicy{}, Options{GitHubAPIBase: "https://127.0.0.1:1"})
	err := c.VerifyToken(context.Background(), GitHub, "github.com", tok)
	if !errors.Is(err, ErrUpstream) || !errors.Is(err, safehttp.ErrBlockedAddress) || strings.Contains(err.Error(), canary) {
		t.Fatal(err)
	}
}

func TestPolicy(t *testing.T) {
	p := HostPolicy{AllowedGitLabHosts: []string{"git.noobee.id", "gitlab.com"}}
	if !p.Allowed(GitHub, "GitHub.com") || p.Allowed(GitHub, "gitlab.com") || !p.Allowed(GitLab, "gitlab.com") ||
		!p.Allowed(GitLab, "git.noobee.id") || p.Allowed(GitLab, "evil.com") || p.Allowed("bitbucket", "bitbucket.org") {
		t.Fatal("policy")
	}
	if got := strings.Join(p.GitLabHosts(), ","); got != "gitlab.com,git.noobee.id" {
		t.Fatal(got)
	}
}
