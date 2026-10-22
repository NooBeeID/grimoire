// Package gitprovider adalah adapter REST GitHub/GitLab (tree + blob) di atas safehttp.
// Tanpa logika domain. Token tidak pernah masuk URL, log, atau pesan error.
package gitprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/infra/config"
	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/secret"
)

const (
	userAgent    = "grimoire-git-sync"
	maxTreeBody  = 16 << 20
	maxErrDrain  = 64 << 10
	defaultGHAPI = "https://api.github.com"
)

// Options mengonfigurasi Client.
type Options struct {
	// GitHubAPIBase kosong = https://api.github.com. Hanya diisi untuk test/e2e.
	GitHubAPIBase string
	// InsecureHosts: host:port GitLab yang dijangkau lewat http (test/e2e).
	InsecureHosts []string
}

// Client memanggil REST GitHub/GitLab.
type Client struct {
	http     *http.Client
	policy   HostPolicy
	ghBase   string
	insecure []string
}

// New membuat Client dari http.Client (wajib safehttp di produksi).
func New(httpClient *http.Client, policy HostPolicy, o Options) *Client {
	base := strings.TrimRight(o.GitHubAPIBase, "/")
	if base == "" {
		base = defaultGHAPI
	}
	return &Client{http: httpClient, policy: policy, ghBase: base, insecure: o.InsecureHosts}
}

// NewFromConfig membangun Client dari config (safehttp, policy, override test).
func NewFromConfig(cfg config.Config) *Client {
	hc := safehttp.New(safehttp.Options{
		Timeout:            cfg.GitSyncTimeout,
		InsecureAllowHosts: cfg.GitSyncInsecureTestHosts,
	})
	return New(hc, HostPolicy{AllowedGitLabHosts: cfg.GitSyncAllowedHosts}, Options{
		GitHubAPIBase: cfg.GitSyncGitHubAPIBase,
		InsecureHosts: cfg.GitSyncInsecureTestHosts,
	})
}

// Policy mengembalikan kebijakan host.
func (c *Client) Policy() HostPolicy { return c.policy }

func (c *Client) glBase(host string) string {
	scheme := "https"
	if slices.Contains(c.insecure, host) {
		scheme = "http"
	}
	return scheme + "://" + host + "/api/v4"
}

// VerifyToken memeriksa token terhadap endpoint "user" provider.
func (c *Client) VerifyToken(ctx context.Context, provider Provider, host string, token secret.String) error {
	if !c.policy.Allowed(provider, host) {
		return ErrHostNotAllowed
	}
	var u string
	switch provider {
	case GitHub:
		u = c.ghBase + "/user"
	case GitLab:
		u = c.glBase(host) + "/user"
	}
	resp, err := c.get(ctx, provider, u, token, "")
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// ListTree mengembalikan blob di bawah folder ("" = semua).
func (c *Client) ListTree(ctx context.Context, repo Repo, folder string, token secret.String) ([]TreeEntry, error) {
	if !c.policy.Allowed(repo.Provider, repo.Host) {
		return nil, ErrHostNotAllowed
	}
	folder = strings.Trim(folder, "/")
	switch repo.Provider {
	case GitHub:
		return c.githubTree(ctx, repo, folder, token)
	case GitLab:
		return c.gitlabTree(ctx, repo, folder, token)
	}
	return nil, ErrHostNotAllowed
}

// FetchBlob mengambil isi blob; tooLarge true bila melebihi maxBytes.
func (c *Client) FetchBlob(ctx context.Context, repo Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error) {
	if !c.policy.Allowed(repo.Provider, repo.Host) {
		return nil, false, ErrHostNotAllowed
	}
	var u, accept string
	switch repo.Provider {
	case GitHub:
		u = fmt.Sprintf("%s/repos/%s/git/blobs/%s", c.ghBase, escapePath(repo.Path), escapeSeg(sha))
		accept = "application/vnd.github.raw"
	case GitLab:
		u = fmt.Sprintf("%s/projects/%s/repository/blobs/%s/raw", c.glBase(repo.Host), escapeSeg(repo.Path), escapeSeg(sha))
	default:
		return nil, false, ErrHostNotAllowed
	}
	resp, err := c.get(ctx, repo.Provider, u, token, accept)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	body, tooLarge, err := safehttp.ReadLimited(resp, maxBytes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("%w: read body", ErrUpstream)
	}
	return body, tooLarge, nil
}

// get mengirim GET dan memetakan status non-2xx ke error sentinel (body dibuang).
func (c *Client) get(ctx context.Context, p Provider, u string, token secret.String, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: bad request", ErrUpstream)
	}
	req.Header.Set("User-Agent", userAgent)
	switch p {
	case GitHub:
		req.Header.Set("Authorization", "Bearer "+token.Reveal())
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if accept == "" {
			accept = "application/vnd.github+json"
		}
	case GitLab:
		req.Header.Set("PRIVATE-TOKEN", token.Reveal())
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrUpstream, scrub(err))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer drain(resp)
	return nil, mapStatus(resp)
}

// scrub membuang detail URL dari error transport; hanya sentinel yang dipertahankan.
func scrub(err error) error {
	for _, s := range []error{safehttp.ErrBlockedAddress, safehttp.ErrSchemeNotAllowed, safehttp.ErrCrossHostRedirect, safehttp.ErrTooManyRedirects} {
		if errorsIs(err, s) {
			return s
		}
	}
	return fmt.Errorf("network error")
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrDrain))
	_ = resp.Body.Close()
}

func mapStatus(resp *http.Response) error {
	h := resp.Header
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && h.Get("X-RateLimit-Remaining") == "0",
		resp.StatusCode == http.StatusForbidden && h.Get("Retry-After") != "":
		return &RateLimitedError{RetryAfter: retryAfter(h)}
	case resp.StatusCode == http.StatusForbidden:
		return ErrForbidden
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	}
	return fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
}

func retryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if d := time.Until(time.Unix(n, 0)); d > 0 {
				return d
			}
		}
	}
	return 0
}
