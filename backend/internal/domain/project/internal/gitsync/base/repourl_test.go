package base

import (
	"strings"
	"testing"

	"grimoire/lib/common/errs"
)

func policy(p, h string) bool {
	switch p {
	case "github":
		return h == "github.com"
	case "gitlab":
		return h == "gitlab.com" || h == "git.noobee.id"
	}
	return false
}

func TestParseRepoURL_OK(t *testing.T) {
	cases := []struct {
		name, url, branch, folder string
		want                      RepoRef
	}{
		{"github tree", "https://github.com/noobee/grimoire-docs/tree/main/docs", "", "",
			RepoRef{"github", "github.com", "noobee/grimoire-docs", "main", "docs", "https://github.com/noobee/grimoire-docs/tree/main/docs"}},
		{"github override tanpa tree", "https://github.com/noobee/grimoire-docs", "main", "",
			RepoRef{"github", "github.com", "noobee/grimoire-docs", "main", "", "https://github.com/noobee/grimoire-docs/tree/main"}},
		{"suffix .git", "https://github.com/noobee/grimoire-docs.git", "main", "api",
			RepoRef{"github", "github.com", "noobee/grimoire-docs", "main", "api", "https://github.com/noobee/grimoire-docs/tree/main/api"}},
		{"www", "https://www.github.com/a/b/tree/dev", "", "",
			RepoRef{"github", "github.com", "a/b", "dev", "", "https://github.com/a/b/tree/dev"}},
		{"override branch bertingkat", "https://github.com/a/b/tree/feature/x/docs", "feature/x", "",
			RepoRef{"github", "github.com", "a/b", "feature/x", "docs", "https://github.com/a/b/tree/feature/x/docs"}},
		{"tanpa override", "https://github.com/a/b/tree/feature/x/docs", "", "",
			RepoRef{"github", "github.com", "a/b", "feature", "x/docs", "https://github.com/a/b/tree/feature/x/docs"}},
		{"override tidak cocok", "https://github.com/a/b/tree/main/docs", "dev", "",
			RepoRef{"github", "github.com", "a/b", "dev", "", "https://github.com/a/b/tree/dev"}},
		{"folder override menang", "https://github.com/a/b/tree/main/docs", "", "/other/dir/",
			RepoRef{"github", "github.com", "a/b", "main", "other/dir", "https://github.com/a/b/tree/main/other/dir"}},
		{"gitlab subgroup", "https://gitlab.com/grp/sub/repo/-/tree/main/api", "", "",
			RepoRef{"gitlab", "gitlab.com", "grp/sub/repo", "main", "api", "https://gitlab.com/grp/sub/repo/-/tree/main/api"}},
		{"gitlab tanpa tree", "https://git.noobee.id/grp/repo", "main", "",
			RepoRef{"gitlab", "git.noobee.id", "grp/repo", "main", "", "https://git.noobee.id/grp/repo/-/tree/main"}},
		{"host uppercase", "https://GitLab.com/g/r.git/-/tree/main", "", "",
			RepoRef{"gitlab", "gitlab.com", "g/r", "main", "", "https://gitlab.com/g/r/-/tree/main"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseRepoURL(c.url, c.branch, c.folder, policy)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestParseRepoURL_Errors(t *testing.T) {
	cases := []struct {
		name, url, branch, folder string
		code                      errs.Code
	}{
		{"tanpa branch", "https://github.com/noobee/grimoire-docs", "", "", CodeInvalidRepoURL},
		{"blob", "https://github.com/a/b/blob/main/x.md", "", "", CodeInvalidRepoURL},
		{"bitbucket", "https://bitbucket.org/x/y", "main", "", errs.CodeHostNotAllowed},
		{"gitlab tak diizinkan", "https://git.contoh.com/x/y", "main", "", errs.CodeHostNotAllowed},
		{"http", "http://github.com/a/b", "main", "", CodeInvalidRepoURL},
		{"userinfo", "https://tok@github.com/a/b", "main", "", CodeInvalidRepoURL},
		{"query", "https://github.com/a/b?x=1", "main", "", CodeInvalidRepoURL},
		{"fragment", "https://github.com/a/b#frag", "main", "", CodeInvalidRepoURL},
		{"dotdot di path", "https://github.com/a/b/tree/main/../x", "", "", CodeInvalidRepoURL},
		{"folder override dotdot", "https://github.com/a/b", "main", "a/../b", CodeInvalidRepoURL},
		{"terlalu panjang", "https://github.com/a/b/" + strings.Repeat("x", 2100), "main", "", CodeInvalidRepoURL},
		{"branch diawali -", "https://github.com/a/b", "-x", "", CodeInvalidRepoURL},
		{"branch ..", "https://github.com/a/b", "a..b", "", CodeInvalidRepoURL},
		{"branch spasi", "https://github.com/a/b", "a b", "", CodeInvalidRepoURL},
		{"satu segmen", "https://github.com/a", "main", "", CodeInvalidRepoURL},
		{"gitlab satu segmen", "https://gitlab.com/a", "main", "", CodeInvalidRepoURL},
		{"kosong", "  ", "main", "", CodeInvalidRepoURL},
		{"path lain", "https://github.com/a/b/issues", "main", "", CodeInvalidRepoURL},
		{"folder terlalu panjang", "https://github.com/a/b", "main", strings.Repeat("a/", 600), CodeInvalidRepoURL},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseRepoURL(c.url, c.branch, c.folder, policy)
			e, ok := err.(*errs.Error)
			if !ok || e.Code != c.code {
				t.Fatalf("err=%v want %s", err, c.code)
			}
			if strings.Contains(e.Message, "tok") && c.name == "userinfo" {
				t.Fatalf("pesan memuat userinfo: %s", e.Message)
			}
		})
	}
}

func TestParseRepoURL_BranchFieldDetail(t *testing.T) {
	_, err := ParseRepoURL("https://github.com/a/b", "", "", policy)
	e := err.(*errs.Error)
	if e.Message != "isi Branch" || e.Details.(map[string]any)["field"] != "branch" {
		t.Fatalf("%+v", e)
	}
}

func TestParseRepoURL_NilPolicyRejects(t *testing.T) {
	if _, err := ParseRepoURL("https://github.com/a/b", "main", "", nil); err == nil {
		t.Fatal("policy nil harus menolak")
	}
}
