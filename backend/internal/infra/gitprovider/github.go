package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/secret"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func escapeSeg(s string) string { return url.PathEscape(s) }

// escapePath meng-escape tiap segmen "a/b/c" tetapi mempertahankan pemisah.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func (c *Client) githubTree(ctx context.Context, repo Repo, folder string, token secret.String) ([]TreeEntry, error) {
	u := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", c.ghBase, escapePath(repo.Path), url.PathEscape(repo.Branch))
	resp, err := c.get(ctx, GitHub, u, token, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, tooLarge, err := safehttp.ReadLimited(resp, maxTreeBody)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: read tree", ErrUpstream)
	}
	if tooLarge {
		return nil, ErrTreeTooLarge
	}
	var doc struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size *int64 `json:"size"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: invalid tree json", ErrUpstream)
	}
	if doc.Truncated {
		return nil, ErrTreeTooLarge
	}
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}
	var out []TreeEntry
	for _, e := range doc.Tree {
		if e.Type != "blob" || !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		size := int64(-1)
		if e.Size != nil {
			size = *e.Size
		}
		out = append(out, TreeEntry{Path: e.Path, Size: size, BlobSHA: e.SHA})
	}
	return out, nil
}
