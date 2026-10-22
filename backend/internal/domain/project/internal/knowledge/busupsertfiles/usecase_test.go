package busupsertfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/ingest"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

const (
	pidA = "0192f5a0-1111-7aaa-8bbb-000000000001"
	uid  = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
)

type fakeRepo struct {
	log      *[]string
	count    int
	existing map[string][]byte
	insErr   error
	inserted []base.File
	updated  []base.File
	inTxN    int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(ingest.Tx) error) error {
	f.inTxN++
	*f.log = append(*f.log, "tx-begin")
	if err := fn(f); err != nil {
		*f.log = append(*f.log, "tx-rollback")
		return err
	}
	*f.log = append(*f.log, "tx-commit")
	return nil
}
func (f *fakeRepo) LockProject(context.Context, string) error { return nil }
func (f *fakeRepo) CountFiles(context.Context, string) (int, error) {
	return f.count, nil
}
func (f *fakeRepo) ExistingHashes(context.Context, string, []string) (map[string][]byte, error) {
	return f.existing, nil
}
func (f *fakeRepo) InsertFiles(_ context.Context, fs []base.File) error {
	if f.insErr != nil {
		return f.insErr
	}
	f.inserted = append(f.inserted, fs...)
	return nil
}
func (f *fakeRepo) UpdateFiles(_ context.Context, fs []base.File) error {
	f.updated = append(f.updated, fs...)
	return nil
}

type fakeAudit struct {
	log    *[]string
	events []audit.Event
}

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) {
	*f.log = append(*f.log, "audit")
	f.events = append(f.events, ev)
}

var lim = base.Limits{MaxFileBytes: 1000, MaxUploadBytes: 100000, MaxUploadFiles: 100, MaxProjectFiles: 5000}

func setup(l base.Limits) (*Usecase, *fakeRepo, *fakeAudit, *[]string) {
	log := &[]string{}
	r := &fakeRepo{log: log, existing: map[string][]byte{}}
	a := &fakeAudit{log: log}
	return NewUsecase(r, a, l), r, a, log
}

func req(prefix string, items ...upsertfiles.Item) upsertfiles.Request {
	return upsertfiles.Request{
		ProjectID: pidA, TargetPrefix: prefix, Source: upsertfiles.SourceGitSync,
		Actor: upsertfiles.Actor{UserID: uid, Role: "developer", Label: "Rina"},
		Meta:  upsertfiles.Meta{IP: "10.1.2.3", UserAgent: "UA"},
		Items: items,
	}
}

func item(i int, p, c string) upsertfiles.Item {
	return upsertfiles.Item{Index: i, Path: p, Content: []byte(c)}
}

func reasonOf(t *testing.T, err error) any {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("bukan CodeInvalid: %v", err)
	}
	return e.Details.(map[string]any)["reason"]
}

func TestValidation(t *testing.T) {
	long := strings.Repeat("a", 513)
	cases := []struct {
		name   string
		mut    func(*upsertfiles.Request)
		reason any
	}{
		{"project", func(r *upsertfiles.Request) { r.ProjectID = "p1" }, base.ReasonInvalidProjectID},
		{"actor kosong", func(r *upsertfiles.Request) { r.Actor.UserID = "" }, reasonInvalidActor},
		{"actor bukan uuid", func(r *upsertfiles.Request) { r.Actor.UserID = "u1" }, reasonInvalidActor},
		{"source kosong", func(r *upsertfiles.Request) { r.Source = "" }, reasonInvalidSource},
		{"source upload", func(r *upsertfiles.Request) { r.Source = "upload" }, reasonInvalidSource},
		{"prefix traversal", func(r *upsertfiles.Request) { r.TargetPrefix = "a/../b" }, base.ReasonPathTraversal},
		{"prefix dotdot", func(r *upsertfiles.Request) { r.TargetPrefix = ".." }, base.ReasonPathTraversal},
		{"prefix abs", func(r *upsertfiles.Request) { r.TargetPrefix = "/abs" }, base.ReasonPathAbsolute},
		{"prefix empty seg", func(r *upsertfiles.Request) { r.TargetPrefix = "a//b" }, base.ReasonPathEmptySegment},
		{"prefix control", func(r *upsertfiles.Request) { r.TargetPrefix = "a\x01b" }, base.ReasonPathControlChar},
		{"prefix long", func(r *upsertfiles.Request) { r.TargetPrefix = long }, base.ReasonPathTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, r, a, _ := setup(lim)
			rq := req("", item(0, "a.md", "# A"))
			c.mut(&rq)
			_, err := u.Execute(context.Background(), rq)
			if got := reasonOf(t, err); got != c.reason {
				t.Fatalf("reason %v, want %v", got, c.reason)
			}
			if r.inTxN != 0 || len(a.events) != 0 {
				t.Fatal("tidak boleh tulis/audit")
			}
		})
	}
}

func TestEmptyItems(t *testing.T) {
	u, r, a, _ := setup(lim)
	res, err := u.Execute(context.Background(), req("guides"))
	if err != nil || len(res.Items) != 0 || res.Summary != (upsertfiles.Summary{}) {
		t.Fatalf("%+v %v", res, err)
	}
	if r.inTxN != 0 || len(a.events) != 0 {
		t.Fatal("tidak boleh tx/audit")
	}
}

func TestPrefixAppliedAndCreated(t *testing.T) {
	u, r, a, log := setup(lim)
	res, err := u.Execute(context.Background(), req("guides", item(0, "auth/login.md", "# L")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Path != "guides/auth/login.md" || res.Items[0].Status != "created" || res.Summary.Created != 1 {
		t.Fatalf("%+v", res)
	}
	if len(r.inserted) != 1 || r.inserted[0].Path != "guides/auth/login.md" || r.inserted[0].CreatedBy != uid {
		t.Fatalf("%+v", r.inserted)
	}
	if got := strings.Join(*log, ","); got != "tx-begin,tx-commit,audit" {
		t.Fatal(got)
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeUploadCompleted || ev.ProjectID != pidA || ev.Actor.ID != uid ||
		ev.Actor.Label != "Rina" || ev.Meta.IP != "10.1.2.3" || ev.Meta.UserAgent != "UA" {
		t.Fatalf("%+v", ev)
	}
	if ev.Args["files"] != 1 || ev.Args["source"] != "git_sync" || ev.Result["created"] != 1 {
		t.Fatalf("%+v", ev)
	}
}

func TestNoPrefix(t *testing.T) {
	u, r, _, _ := setup(lim)
	res, err := u.Execute(context.Background(), req("", item(0, "a.md", "# A")))
	if err != nil || res.Items[0].Path != "a.md" || r.inserted[0].Path != "a.md" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUnchangedAndUpdated(t *testing.T) {
	u, r, _, _ := setup(lim)
	same := base.NewFile(pidA, "guides/a.md", "# A", base.Actor{ID: uid})
	r.existing["guides/a.md"] = same.Hash
	r.existing["guides/b.md"] = []byte("old-hash")
	res, err := u.Execute(context.Background(), req("guides", item(0, "a.md", "# A"), item(1, "b.md", "# B2")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != "unchanged" || res.Items[1].Status != "updated" ||
		res.Summary.Unchanged != 1 || res.Summary.Updated != 1 {
		t.Fatalf("%+v", res)
	}
	if len(r.inserted) != 0 || len(r.updated) != 1 || r.updated[0].Path != "guides/b.md" {
		t.Fatalf("%+v %+v", r.inserted, r.updated)
	}
}

func TestRejectionsAndOrder(t *testing.T) {
	u, r, _, _ := setup(lim)
	res, err := u.Execute(context.Background(), req("",
		item(4, "z.md", "# Z"),
		item(3, "legacy/../x.md", "x"),
		item(2, ".github/notes.md", "x"),
		item(1, "img/x.png", "x"),
		upsertfiles.Item{Index: 5, Path: "big.md", TooLarge: true},
		item(6, "huge.md", strings.Repeat("a", 1001)),
		item(7, "bad.md", "a\x00b"),
		item(0, "a.md", "# A"),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		st, reason string
	}{
		{"created", ""}, {"rejected", "unsupported_type"}, {"skipped", "hidden"}, {"rejected", "path_traversal"},
		{"created", ""}, {"rejected", "too_large"}, {"rejected", "too_large"}, {"rejected", "invalid_encoding"},
	}
	if len(res.Items) != len(want) {
		t.Fatalf("%+v", res.Items)
	}
	for i, w := range want {
		if res.Items[i].Index != i || res.Items[i].Status != w.st || res.Items[i].Reason != w.reason {
			t.Fatalf("item %d: %+v want %+v", i, res.Items[i], w)
		}
	}
	if len(r.inserted) != 2 {
		t.Fatalf("%+v", r.inserted)
	}
}

func TestDuplicateAfterPrefix(t *testing.T) {
	u, _, _, _ := setup(lim)
	res, err := u.Execute(context.Background(), req("g", item(0, "a.md", "1"), item(1, "a.md", "2")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[1].Status != "rejected" || res.Items[1].Reason != "duplicate_in_request" {
		t.Fatalf("%+v", res.Items)
	}
}

func TestProjectFileLimit(t *testing.T) {
	l := lim
	l.MaxProjectFiles = 3
	u, r, _, _ := setup(l)
	r.count = 2
	r.existing["old.md"] = []byte("old")
	res, err := u.Execute(context.Background(), req("",
		item(0, "old.md", "new"), item(1, "n1.md", "1"), item(2, "n2.md", "2"), item(3, "n3.md", "3")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != "updated" || res.Summary.Created != 1 || res.Summary.Rejected != 2 {
		t.Fatalf("%+v", res)
	}
	if res.Items[3].Reason != "project_file_limit" {
		t.Fatalf("%+v", res.Items)
	}
}

func TestAllRejectedSkipsTxButAudits(t *testing.T) {
	u, r, a, _ := setup(lim)
	res, err := u.Execute(context.Background(), req("", item(0, "x.png", "x"), item(1, "../y.md", "y")))
	if err != nil {
		t.Fatal(err)
	}
	if r.inTxN != 0 || res.Summary.Rejected != 2 || len(a.events) != 1 || a.events[0].Result["rejected"] != 2 {
		t.Fatalf("%+v %+v", res, a.events)
	}
}

func TestAuditPathsTruncatedAndNoContent(t *testing.T) {
	u, _, a, _ := setup(lim)
	var items []upsertfiles.Item
	for i := 0; i < 25; i++ {
		items = append(items, item(i, fmt.Sprintf("f%02d.md", i), "RAHASIA-ISI"))
	}
	if _, err := u.Execute(context.Background(), req("", items...)); err != nil {
		t.Fatal(err)
	}
	ev := a.events[0]
	if len(ev.Result["paths"].([]string)) != 20 || ev.Result["paths_truncated"] != true {
		t.Fatalf("%+v", ev.Result)
	}
	if s := fmt.Sprint(ev.Args, ev.Result); strings.Contains(s, "RAHASIA-ISI") {
		t.Fatal("isi file bocor ke audit")
	}
}

func TestRepoErrorNoAudit(t *testing.T) {
	u, r, a, _ := setup(lim)
	r.insErr = errors.New("boom")
	_, err := u.Execute(context.Background(), req("", item(0, "a.md", "# A")))
	if err == nil || !strings.Contains(err.Error(), "busupsertfiles: batch:") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("%v", err)
	}
	if len(a.events) != 0 {
		t.Fatal("tidak boleh audit")
	}
}
