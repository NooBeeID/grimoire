package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/config"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func TestParseKindAndExt(t *testing.T) {
	if k, err := ParseKind("knowledge"); err != nil || k != KindKnowledge {
		t.Fatal(k, err)
	}
	if k, err := ParseKind("api_spec"); err != nil || k != KindAPISpec {
		t.Fatal(k, err)
	}
	for _, in := range []string{"", "Knowledge", "apispec"} {
		if _, err := ParseKind(in); err != ErrInvalidKind {
			t.Errorf("%q", in)
		}
	}
	if !KindKnowledge.AllowedExt("a/B.MD") || !KindKnowledge.AllowedExt("x.txt") || KindKnowledge.AllowedExt("x.png") || KindKnowledge.AllowedExt("md") {
		t.Fatal("knowledge ext")
	}
	if !KindAPISpec.AllowedExt("a.YAML") || !KindAPISpec.AllowedExt("a.yml") || !KindAPISpec.AllowedExt("a.json") || KindAPISpec.AllowedExt("a.md") {
		t.Fatal("api ext")
	}
	if KindKnowledge.SkipReason() != "unsupported_type" || KindAPISpec.SkipReason() != "NOT_SPEC_FILE" {
		t.Fatal("skip reason")
	}
}

func TestNormalizeTargetPrefix(t *testing.T) {
	for in, want := range map[string]string{"": "", "/guides/": "guides", " a//b ": "a/b"} {
		if got, err := NormalizeTargetPrefix(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"a/../b", "a/./b", "a\x00b", strings.Repeat("a", 513), "\xff", "a\nb"} {
		if _, err := NormalizeTargetPrefix(in); err != ErrInvalidTargetPrefix {
			t.Errorf("%q", in)
		}
	}
	if _, err := NormalizeTargetPrefix(strings.Repeat("a", 512)); err != nil {
		t.Error("512 lolos")
	}
}

func TestSelectFiles(t *testing.T) {
	entries := []TreeEntry{
		{"docs/b/z.txt", 3, "s3"}, {"docs/a.md", 1, "s1"}, {"docs/img/x.png", 5, "s2"},
		{"docsx/c.md", 1, "s4"}, {"README.md", 1, "s5"},
	}
	c, sk, err := SelectFiles(entries, "docs", KindKnowledge)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c[0].RepoPath != "docs/a.md" || c[0].RelPath != "a.md" || c[1].RelPath != "b/z.txt" || c[1].SHA != "s3" {
		t.Fatalf("%+v", c)
	}
	if len(sk) != 1 || sk[0].RepoPath != "docs/img/x.png" || sk[0].Status != ItemSkipped || sk[0].ReasonCode != "unsupported_type" {
		t.Fatalf("%+v", sk)
	}
	all, sk, _ := SelectFiles(entries, "", KindKnowledge)
	if len(all) != 4 || len(sk) != 1 || all[0].RepoPath != "README.md" {
		t.Fatalf("%+v %+v", all, sk)
	}
	if _, _, err := SelectFiles(entries, "tidak-ada", KindKnowledge); err != ErrFolderNotFound {
		t.Fatal(err)
	}
	if _, _, err := SelectFiles(nil, "", KindKnowledge); err != nil {
		t.Fatal("root kosong bukan error")
	}
	c, sk, _ = SelectFiles([]TreeEntry{{"api/a.YAML", 1, "x"}, {"api/b.md", 1, "y"}}, "api", KindAPISpec)
	if len(c) != 1 || c[0].RelPath != "a.YAML" || len(sk) != 1 || sk[0].ReasonCode != "NOT_SPEC_FILE" {
		t.Fatalf("%+v %+v", c, sk)
	}
}

func TestCheckBatchLimits(t *testing.T) {
	l := Limits{MaxFileBytes: 10, MaxFiles: 2, MaxTotalBytes: 10}
	mk := func(sizes ...int64) []Candidate {
		var out []Candidate
		for _, s := range sizes {
			out = append(out, Candidate{Size: s})
		}
		return out
	}
	if err := CheckBatchLimits(mk(5, 5), l); err != nil {
		t.Fatal("tepat batas lolos", err)
	}
	e := CheckBatchLimits(mk(1, 1, 1), l).(*errs.Error)
	if e.Code != CodeSyncLimitExceeded || e.Details.(map[string]any)["limit"] != "files" {
		t.Fatal(e)
	}
	e = CheckBatchLimits(mk(5, 6), l).(*errs.Error)
	if e.Code != CodeSyncLimitExceeded || e.Details.(map[string]any)["limit"] != "bytes" {
		t.Fatal(e)
	}
	if err := CheckBatchLimits(mk(-1, -1), l); err != nil {
		t.Fatal("size -1 diabaikan")
	}
	if CheckFetchedTotal(10, l) != nil || CheckFetchedTotal(11, l) == nil {
		t.Fatal("fetched total")
	}
}

func TestLimitsFor(t *testing.T) {
	cfg := config.Config{MaxFileBytes: 1, MaxUploadFiles: 500, MaxUploadBytes: 50, APISpecMaxFileBytes: 5, APISpecMaxSpecs: 100}
	if l := LimitsFor(KindKnowledge, cfg); l != (Limits{1, 500, 50}) {
		t.Fatal(l)
	}
	if l := LimitsFor(KindAPISpec, cfg); l != (Limits{5, 100, 50}) {
		t.Fatal(l)
	}
	cfg.APISpecMaxSpecs = 1000
	if LimitsFor(KindAPISpec, cfg).MaxFiles != 500 {
		t.Fatal("min")
	}
}

func TestReport(t *testing.T) {
	r := NewReport([]ReportItem{
		{RepoPath: "b", Status: ItemCreated}, {RepoPath: "a", Status: ItemSkipped}, {RepoPath: "c", Status: ItemUnchanged}, {RepoPath: "d", Status: ItemUpdated},
	})
	if r.Items[0].RepoPath != "a" || r.Status != StatusSuccess || r.Summary != (Summary{Created: 1, Updated: 1, Unchanged: 1, Skipped: 1}) {
		t.Fatalf("%+v", r)
	}
	r = NewReport([]ReportItem{{RepoPath: "a", Status: ItemRejected}})
	if r.Status != StatusPartial {
		t.Fatal(r.Status)
	}
	if NewReport(nil).Status != StatusSuccess {
		t.Fatal("nol item = success")
	}
	f := FailedReport("GIT_NOT_FOUND", "x")
	if f.Status != StatusFailed || f.ErrorCode != "GIT_NOT_FOUND" {
		t.Fatal(f)
	}
	r = NewReport([]ReportItem{{RepoPath: "a", Status: ItemCreated}, {RepoPath: "b", Status: ItemCreated}, {RepoPath: "c", Status: ItemRejected}})
	c := r.CapForStorage(2)
	if len(c.Items) != 2 || !c.Truncated || c.Summary.Created != 2 || c.Summary.Rejected != 1 || len(r.Items) != 3 || r.Truncated {
		t.Fatalf("%+v / %+v", c, r)
	}
	if r.CapForStorage(3).Truncated {
		t.Fatal("tidak dipotong")
	}
}

func TestReportJSON(t *testing.T) {
	r := NewReport([]ReportItem{{RepoPath: "a", Status: ItemCreated}})
	b, _ := json.Marshal(r.ToDTO())
	s := string(b)
	if strings.Contains(strings.ToLower(s), "token") || !strings.Contains(s, `"issues":[]`) || strings.Contains(s, "openapi_version") {
		t.Fatal(s)
	}
	var d ReportDTO
	if err := json.Unmarshal(b, &d); err != nil || d.ToReport().Summary.Created != 1 {
		t.Fatal(err)
	}
}

func TestNewConfigAndDTO(t *testing.T) {
	ref := RepoRef{Provider: "github", Host: "github.com", Path: "a/b", Branch: "main", Folder: "docs", CanonicalURL: "https://github.com/a/b/tree/main/docs"}
	c := NewConfig("p1", KindKnowledge, ref, "guides")
	if c.TargetPrefix != "guides" || c.RepoURL != ref.CanonicalURL {
		t.Fatal(c)
	}
	if NewConfig("p1", KindAPISpec, ref, "guides").TargetPrefix != "" {
		t.Fatal("prefix api_spec harus kosong")
	}
	if c.ToDTO().LastSyncedBy != nil || c.ToDTO().LastResult != nil {
		t.Fatal("belum pernah sync")
	}
	now := time.Now()
	st, uid, lbl := "partial", "u1", "Rina"
	rep := NewReport(nil)
	c.LastStatus, c.LastSyncedAt, c.LastSyncedBy, c.LastSyncedByLabel, c.LastResult = &st, &now, &uid, &lbl, &rep
	d := c.ToDTO()
	if d.LastSyncedBy.Label != "Rina" || d.LastResult == nil || *d.LastStatus != "partial" {
		t.Fatal(d)
	}
	b, _ := json.Marshal(d)
	for _, k := range []string{"repo_url", "repo_path", "target_prefix", "last_synced_at", "last_synced_by", "last_result"} {
		if !strings.Contains(string(b), `"`+k+`"`) {
			t.Errorf("kunci %s hilang", k)
		}
	}
}

func TestActorMetaErrors(t *testing.T) {
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Fatal()
	}
	a, _ := ActorFrom(reqctx.Identity{UserID: "u", Role: "developer", Name: " Rina ", Email: "e"}, true)
	if a != (Actor{ID: "u", Role: "developer", Label: "Rina"}) {
		t.Fatal(a)
	}
	if m := NewRequestMeta("ip", strings.Repeat("é", 400)); len(m.UserAgent) > 512 {
		t.Fatal()
	}
	e := CredentialMissing("github", "github.com")
	if e.Details.(map[string]any)["settings_path"] != "/profile#token-git" || e.Code != CodeCredentialMissing {
		t.Fatal(e)
	}
	if SyncInProgress(KindKnowledge).Details.(map[string]any)["kind"] != "knowledge" {
		t.Fatal()
	}
	if f := FolderNotFound("x"); f.Code != CodeGitNotFound || f.Details.(map[string]any)["folder"] != "x" {
		t.Fatal(f)
	}
	if ErrFolderNotFound.Code != CodeGitNotFound {
		t.Fatal(ErrFolderNotFound)
	}
}
