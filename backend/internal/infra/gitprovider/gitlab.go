package gitprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/secret"
)

const maxGitLabPages = 50

var linkNextRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="?next"?`)

func (c *Client) gitlabTree(ctx context.Context, get getFn, repo Repo, folder string, token secret.String) ([]TreeEntry, error) {
	base := c.glBase(repo.Host)
	q := url.Values{}
	q.Set("path", folder)
	q.Set("ref", repo.Branch)
	q.Set("recursive", "true")
	q.Set("per_page", "100")
	q.Set("pagination", "keyset")
	next := fmt.Sprintf("%s/projects/%s/repository/tree?%s", base, escapeSeg(repo.Path), q.Encode())
	wantHost := ""
	if u, err := url.Parse(next); err == nil {
		wantHost = u.Host
	}
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}
	var out []TreeEntry
	for page := 0; next != ""; page++ {
		if page >= maxGitLabPages {
			return nil, ErrTreeTooLarge
		}
		resp, err := get(ctx, GitLab, next, token, "")
		if err != nil {
			return nil, err
		}
		body, tooLarge, rerr := safehttp.ReadLimited(resp, maxTreeBody)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if rerr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: read tree", ErrUpstream)
		}
		if tooLarge {
			return nil, ErrTreeTooLarge
		}
		var items []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("%w: invalid tree json", ErrUpstream)
		}
		for _, e := range items {
			if e.Type == "blob" && strings.HasPrefix(e.Path, prefix) {
				out = append(out, TreeEntry{Path: e.Path, Size: -1, BlobSHA: e.ID})
			}
		}
		next = ""
		if m := linkNextRe.FindStringSubmatch(link); m != nil {
			// Hanya ikuti bila host sama dengan host API.
			if u, err := url.Parse(m[1]); err == nil && u.Host == wantHost && u.Host != "" {
				next = u.String()
			}
		}
	}
	return out, nil
}
