package gitprovider

import (
	"slices"
	"strings"
)

// Provider adalah jenis penyedia Git.
type Provider string

const (
	GitHub Provider = "github"
	GitLab Provider = "gitlab"
)

// HostPolicy menentukan host yang boleh dihubungi.
type HostPolicy struct {
	AllowedGitLabHosts []string // lowercase, termasuk port bila ada
}

// Allowed: github hanya "github.com"; gitlab "gitlab.com" atau anggota AllowedGitLabHosts.
func (p HostPolicy) Allowed(provider Provider, host string) bool {
	host = strings.ToLower(host)
	switch provider {
	case GitHub:
		return host == "github.com"
	case GitLab:
		return host == "gitlab.com" || slices.Contains(p.AllowedGitLabHosts, host)
	}
	return false
}

// GitLabHosts mengembalikan ["gitlab.com", ...AllowedGitLabHosts] tanpa duplikat.
func (p HostPolicy) GitLabHosts() []string {
	out := []string{"gitlab.com"}
	for _, h := range p.AllowedGitLabHosts {
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

// Repo mengidentifikasi repo dan branch.
type Repo struct {
	Provider Provider
	Host     string // github.com | gitlab.com | host self-hosted
	Path     string // github "owner/repo"; gitlab "group/sub/repo"
	Branch   string
}

// TreeEntry adalah satu file (blob) di tree.
type TreeEntry struct {
	Path    string
	Size    int64 // -1 bila tidak diketahui
	BlobSHA string
}
