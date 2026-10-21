package uploadfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
)

var actor = base.Actor{ID: "u1", Role: "developer", Label: "Budi"}

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	if f.err != nil {
		return base.Actor{}, f.err
	}
	return actor, nil
}

type fakeRepo struct {
	log      *[]string
	count    int
	existing map[string][]byte
	insErr   error
	inserted []base.File
	updated  []base.File
	hashArgs []string
	inTxN    int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(repository) error) error {
	f.inTxN++
	*f.log = append(*f.log, "tx-begin")
	if err := fn(f); err != nil {
		*f.log = append(*f.log, "tx-rollback")
		return err
	}
	*f.log = append(*f.log, "tx-commit")
	return nil
}
func (f *fakeRepo) LockProject(context.Context, string) error {
	*f.log = append(*f.log, "lock")
	return nil
}
func (f *fakeRepo) CountFiles(context.Context, string) (int, error) {
	*f.log = append(*f.log, "count")
	return f.count, nil
}
func (f *fakeRepo) ExistingHashes(_ context.Context, _ string, paths []string) (map[string][]byte, error) {
	*f.log = append(*f.log, "hashes")
	f.hashArgs = paths
	return f.existing, nil
}
func (f *fakeRepo) InsertFiles(_ context.Context, fs []base.File) error {
	*f.log = append(*f.log, "insert")
	if f.insErr != nil {
		return f.insErr
	}
	f.inserted = append(f.inserted, fs...)
	return nil
}
func (f *fakeRepo) UpdateFiles(_ context.Context, fs []base.File) error {
	*f.log = append(*f.log, "update")
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

func setup(lim base.Limits) (*Usecase, *fakeRepo, *fakeAudit, *[]string) {
	log := &[]string{}
	r := &fakeRepo{log: log, existing: map[string][]byte{}}
	a := &fakeAudit{log: log}
	return NewUsecase(r, fakeAccess{}, a, lim), r, a, log
}

var ulim = base.Limits{MaxFileBytes: 1000, MaxUploadBytes: 100000, MaxUploadFiles: 100, MaxProjectFiles: 5000}

func part(i int, name, data string) RawPart { return RawPart{Index: i, Name: name, Data: []byte(data)} }

func in(parts ...RawPart) Input { return Input{ProjectID: "p1", Parts: parts} }

func TestNoFiles(t *testing.T) {
	u, r, a, _ := setup(ulim)
	_, err := u.Execute(context.Background(), actor, in())
	var e *errs.Error
	if !errors.As(err, &e) || e.Details.(map[string]any)["reason"] != base.ReasonNoFiles {
		t.Fatalf("%v", err)
	}
	if r.inTxN != 0 || len(a.events) != 0 {
		t.Fatal("tidak boleh menyentuh repo/audit")
	}
}

func TestAllRejectedSkipsDBButAudits(t *testing.T) {
	u, r, a, _ := setup(ulim)
	res, err := u.Execute(context.Background(), actor, in(part(0, ".git/config", "[core]"), part(1, "x.png", "x")))
	if err != nil {
		t.Fatal(err)
	}
	if r.inTxN != 0 {
		t.Fatal("InTx tidak boleh dipanggil")
	}
	if res.Summary.Skipped != 1 || res.Summary.Rejected != 1 {
		t.Fatalf("%+v", res.Summary)
	}
	if len(a.events) != 1 || a.events[0].Result["created"] != 0 {
		t.Fatalf("%+v", a.events)
	}
}

func TestMixedOrderAndOverCount(t *testing.T) {
	u, r, _, _ := setup(ulim)
	res, err := u.Execute(context.Background(), actor, in(
		part(0, "a.md", "# A"),
		part(1, "../x.md", "x"),
		part(2, ".DS_Store", "x"),
		RawPart{Index: 3, Name: "late.md", OverCount: true},
		part(4, "b.md", "# B"),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		p string
		s base.Status
		r base.Reason
	}{
		{"a.md", base.StatusCreated, ""},
		{"../x.md", base.StatusRejected, base.ReasonPathTraversal},
		{".DS_Store", base.StatusSkipped, base.ReasonHidden},
		{"late.md", base.StatusRejected, base.ReasonTooManyFiles},
		{"b.md", base.StatusCreated, ""},
	}
	if len(res.Items) != len(want) {
		t.Fatalf("%+v", res.Items)
	}
	for i, w := range want {
		g := res.Items[i]
		if g.Path != w.p || g.Status != w.s || g.Reason != w.r {
			t.Fatalf("item %d = %+v, want %+v", i, g, w)
		}
	}
	if len(r.inserted) != 2 {
		t.Fatalf("inserted %d", len(r.inserted))
	}
}

func TestDBCallOrderAndAuditAfterCommit(t *testing.T) {
	u, _, _, log := setup(ulim)
	r2 := u.repo.(*fakeRepo)
	r2.existing = map[string][]byte{"old.md": base.Hash("lama")}
	if _, err := u.Execute(context.Background(), actor, in(part(0, "new.md", "# N"), part(1, "old.md", "baru"))); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(*log, ",")
	want := "tx-begin,lock,count,hashes,insert,update,tx-commit,audit"
	if got != want {
		t.Fatalf("%s\nwant %s", got, want)
	}
	if len(r2.hashArgs) != 2 {
		t.Fatalf("%v", r2.hashArgs)
	}
	if r2.inserted[0].Title != "N" || r2.inserted[0].UpdatedByLabel != "Budi" || r2.inserted[0].CreatedBy != "u1" {
		t.Fatalf("%+v", r2.inserted[0])
	}
}

func TestUnchangedNotWritten(t *testing.T) {
	u, r, _, _ := setup(ulim)
	r.existing = map[string][]byte{"a.md": base.Hash("# A"), "b.md": base.Hash("lama")}
	res, err := u.Execute(context.Background(), actor, in(part(0, "a.md", "# A"), part(1, "b.md", "baru")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Unchanged != 1 || res.Summary.Updated != 1 || len(r.inserted) != 0 || len(r.updated) != 1 || r.updated[0].Path != "b.md" {
		t.Fatalf("%+v %+v %+v", res.Summary, r.inserted, r.updated)
	}
}

func TestDuplicateInRequest(t *testing.T) {
	u, r, _, _ := setup(ulim)
	res, err := u.Execute(context.Background(), actor, in(part(0, "a.md", "1"), part(1, "a.md", "2")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[1].Reason != base.ReasonDuplicateInRequest || len(r.inserted) != 1 {
		t.Fatalf("%+v", res.Items)
	}
}

func TestQuota(t *testing.T) {
	l := ulim
	l.MaxProjectFiles = 5000
	u, r, _, _ := setup(l)
	r.count = 4998
	res, err := u.Execute(context.Background(), actor, in(part(0, "a.md", "1"), part(1, "b.md", "2"), part(2, "c.md", "3")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Created != 2 || res.Items[2].Reason != base.ReasonProjectFileLimit {
		t.Fatalf("%+v", res)
	}
}

func TestRepoErrorNoAudit(t *testing.T) {
	u, r, a, _ := setup(ulim)
	r.insErr = errors.New("boom")
	_, err := u.Execute(context.Background(), actor, in(part(0, "a.md", "1")))
	var e *errs.Error
	if err == nil || errors.As(err, &e) {
		t.Fatalf("want infra error, got %v", err)
	}
	if len(a.events) != 0 {
		t.Fatal("audit tidak boleh dipanggil")
	}
}

func TestAuthorizeError(t *testing.T) {
	u, r, a, _ := setup(ulim)
	u.access = fakeAccess{err: base.ErrProjectNotFound}
	if _, err := u.Authorize(context.Background(), "p1"); err != base.ErrProjectNotFound {
		t.Fatalf("%v", err)
	}
	if r.inTxN != 0 || len(a.events) != 0 {
		t.Fatal("tidak boleh menyentuh repo/audit")
	}
}

func TestAuditEvent(t *testing.T) {
	u, _, a, _ := setup(ulim)
	var parts []RawPart
	total := 0
	for i := 0; i < 25; i++ {
		parts = append(parts, part(i, fmt.Sprintf("f%02d.md", i), "RAHASIA-123"))
		total += len("RAHASIA-123")
	}
	parts = append(parts, part(25, ".git/config", "x"))
	if _, err := u.Execute(context.Background(), actor, in(parts...)); err != nil {
		t.Fatal(err)
	}
	ev := a.events[0]
	if len(a.events) != 1 || ev.Action != record.ActionKnowledgeUploadCompleted || ev.TargetType != "knowledge" || ev.Target != "" {
		t.Fatalf("%+v", ev)
	}
	if ev.Args["files"] != 26 || ev.Result["created"] != 25 || ev.Result["skipped"] != 1 || ev.Result["bytes"] != total {
		t.Fatalf("%+v %+v", ev.Args, ev.Result)
	}
	if p := ev.Result["paths"].([]string); len(p) != 20 || ev.Result["paths_truncated"] != true {
		t.Fatalf("%+v", ev.Result)
	}
	if s := fmt.Sprint(ev.Args, ev.Result); strings.Contains(s, "RAHASIA") {
		t.Fatal("isi file bocor ke audit")
	}
}
