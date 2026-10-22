package base

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxURLBytes    = 2048
	maxBranchBytes = 255
	maxFolderBytes = 1024
)

// RepoRef adalah hasil parse URL repo yang sudah tervalidasi.
type RepoRef struct {
	Provider     string // "github" | "gitlab"
	Host         string
	Path         string // github "owner/repo"; gitlab "group/sub/repo"
	Branch       string
	Folder       string // tanpa "/" depan/belakang; "" = root
	CanonicalURL string
}

// ParseRepoURL mengurai URL repo/folder (K3). Pesan error tidak pernah memuat string mentah user.
// allowed(provider, host) mengabstraksi kebijakan host (gitprovider.HostPolicy).
func ParseRepoURL(raw, branchOverride, folderOverride string, allowed func(provider, host string) bool) (RepoRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxURLBytes {
		return RepoRef{}, InvalidRepoURL("URL repo tidak valid", "url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return RepoRef{}, InvalidRepoURL("URL repo tidak valid", "url")
	}
	if u.Scheme != "https" {
		return RepoRef{}, InvalidRepoURL("hanya URL https yang didukung", "url")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Host == "" {
		return RepoRef{}, InvalidRepoURL("URL tidak boleh berisi userinfo, query, atau fragment", "url")
	}

	provider, host := "gitlab", strings.ToLower(u.Host)
	if host == "github.com" || host == "www.github.com" {
		provider, host = "github", "github.com"
	}
	if allowed == nil || !allowed(provider, host) {
		return RepoRef{}, HostNotAllowed(host)
	}

	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s == "" {
			continue
		}
		if s == "." || s == ".." || hasControl(s) {
			return RepoRef{}, InvalidRepoURL("URL repo tidak valid", "url")
		}
		segs = append(segs, s)
	}

	var proj, rest []string
	if provider == "github" {
		if len(segs) < 2 {
			return RepoRef{}, InvalidRepoURL("URL harus berbentuk https://github.com/{owner}/{repo}", "url")
		}
		proj, rest = segs[:2], segs[2:]
	} else {
		proj, rest = segs, nil
		for i, s := range segs {
			if s == "-" {
				proj, rest = segs[:i], segs[i+1:]
				break
			}
		}
		if len(proj) < 2 {
			return RepoRef{}, InvalidRepoURL("URL harus berbentuk https://{host}/{group}/{repo}", "url")
		}
	}
	proj = append([]string(nil), proj...)
	proj[len(proj)-1] = strings.TrimSuffix(proj[len(proj)-1], ".git")
	if proj[len(proj)-1] == "" {
		return RepoRef{}, InvalidRepoURL("URL repo tidak valid", "url")
	}

	var after []string
	if len(rest) > 0 {
		switch rest[0] {
		case "tree":
			after = rest[1:]
		case "blob":
			return RepoRef{}, InvalidRepoURL("gunakan URL folder (tree), bukan URL file (blob)", "url")
		default:
			return RepoRef{}, InvalidRepoURL("URL repo tidak valid", "url")
		}
	}

	branch := strings.TrimSpace(branchOverride)
	var urlFolder []string
	if branch != "" {
		if len(after) > 0 {
			bs := strings.Split(branch, "/")
			if hasPrefixSegs(after, bs) {
				urlFolder = after[len(bs):]
			}
		}
	} else {
		if len(after) == 0 {
			return RepoRef{}, InvalidRepoURL("isi Branch", "branch")
		}
		branch, urlFolder = after[0], after[1:]
	}
	if !validBranch(branch) {
		return RepoRef{}, InvalidRepoURL("nama branch tidak valid", "branch")
	}

	folderRaw := strings.Join(urlFolder, "/")
	if f := strings.TrimSpace(folderOverride); f != "" {
		folderRaw = f
	}
	folder, ok := normalizeFolder(folderRaw)
	if !ok {
		return RepoRef{}, InvalidRepoURL("folder tidak valid", "folder")
	}

	path := strings.Join(proj, "/")
	canon := "https://" + host + "/" + path
	if provider == "gitlab" {
		canon += "/-"
	}
	canon += "/tree/" + branch
	if folder != "" {
		canon += "/" + folder
	}
	return RepoRef{Provider: provider, Host: host, Path: path, Branch: branch, Folder: folder, CanonicalURL: canon}, nil
}

func hasPrefixSegs(s, prefix []string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := range prefix {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

func hasControl(s string) bool {
	for _, r := range s {
		if r == 0 || r == '\\' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validBranch(b string) bool {
	if b == "" || len(b) > maxBranchBytes || !utf8.ValidString(b) || strings.HasPrefix(b, "-") || strings.Contains(b, "..") {
		return false
	}
	for _, r := range b {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// normalizeFolder membuang "/" depan/belakang dan segmen kosong; menolak "."/".."/NUL/kontrol.
func normalizeFolder(raw string) (string, bool) {
	if len(raw) > maxFolderBytes || !utf8.ValidString(raw) {
		return "", false
	}
	var segs []string
	for _, s := range strings.Split(raw, "/") {
		switch {
		case s == "":
			continue
		case s == "." || s == "..", hasControl(s):
			return "", false
		}
		segs = append(segs, s)
	}
	return strings.Join(segs, "/"), true
}
