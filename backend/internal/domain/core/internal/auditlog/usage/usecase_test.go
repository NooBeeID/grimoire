package usage

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

var fixedNow = time.Date(2026, 10, 22, 10, 30, 0, 0, time.UTC)

type fakeRepo struct {
	calls                   []string
	topErr, zeroErr, keyErr error
	unreadErr               error
	w                       base.Window
	unreadPaths             []string
	unreadLimit             int
	unreadOut               []string
}

func (f *fakeRepo) TopFiles(_ context.Context, w base.Window, _ string, _ int) ([]base.FileRead, error) {
	f.calls = append(f.calls, "top")
	f.w = w
	return []base.FileRead{{Path: "a.md", Reads: 2}}, f.topErr
}
func (f *fakeRepo) UnreadPaths(_ context.Context, _ base.Window, _ string, paths []string, limit int) ([]string, error) {
	f.calls = append(f.calls, "unread")
	f.unreadPaths, f.unreadLimit = paths, limit
	return f.unreadOut, f.unreadErr
}
func (f *fakeRepo) ZeroQueries(context.Context, base.Window, string, int) ([]base.ZeroQuery, error) {
	f.calls = append(f.calls, "zero")
	return nil, f.zeroErr
}
func (f *fakeRepo) PerKey(context.Context, base.Window, string, int) ([]base.KeyUsage, error) {
	f.calls = append(f.calls, "keys")
	return nil, f.keyErr
}

type fakeFinder struct {
	found bool
	err   error
}

func (f fakeFinder) Exists(context.Context, string) (bool, error) { return f.found, f.err }

type fakeLister struct {
	files  []base.KnownFile
	capped bool
	err    error
	calls  int
}

func (f *fakeLister) ListAll(context.Context, string) ([]base.KnownFile, bool, error) {
	f.calls++
	return f.files, f.capped, f.err
}

func newUC(r *fakeRepo, p fakeFinder, l *fakeLister) (*Usecase, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return NewUsecase(r, p, l, func() time.Time { return fixedNow }, slog.New(slog.NewTextHandler(buf, nil))), buf
}

const pid = "0192f5a0-1111-7aaa-8bbb-000000000001"

func TestUsage_AllSectionsNoProject(t *testing.T) {
	r, l := &fakeRepo{}, &fakeLister{}
	uc, _ := newUC(r, fakeFinder{}, l)
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d})
	if err != nil {
		t.Fatal(err)
	}
	if rep.TopFiles.Available || rep.TopFiles.ErrorCode != base.SectionProjectRequired || len(rep.TopFiles.Items) != 0 {
		t.Fatalf("top: %+v", rep.TopFiles)
	}
	if rep.UnreadFiles.Available || rep.UnreadFiles.ErrorCode != base.SectionProjectRequired {
		t.Fatalf("unread: %+v", rep.UnreadFiles)
	}
	if !rep.ZeroQueries.Available || !rep.PerKey.Available {
		t.Fatal("zero/keys harus available")
	}
	if strings.Join(r.calls, ",") != "zero,keys" || l.calls != 0 {
		t.Fatalf("calls %v lister %d", r.calls, l.calls)
	}
}

func TestT19_AC3_UnknownProject404(t *testing.T) {
	r := &fakeRepo{}
	uc, _ := newUC(r, fakeFinder{found: false}, &fakeLister{})
	_, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid})
	if !errors.Is(err, errs.ErrNotFound) || len(r.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, r.calls)
	}
}

func TestUsage_ProjectLookupErrorIs500(t *testing.T) {
	uc, _ := newUC(&fakeRepo{}, fakeFinder{err: errors.New("bus down")}, &fakeLister{})
	_, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid})
	if err == nil || errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestUsage_SectionFailSoft(t *testing.T) {
	r := &fakeRepo{zeroErr: errors.New("boom")}
	uc, buf := newUC(r, fakeFinder{found: true}, &fakeLister{})
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	z := rep.ZeroQueries
	if z.Available || z.ErrorCode != base.SectionUnavailable || z.Items == nil || len(z.Items) != 0 {
		t.Fatalf("zero: %+v", z)
	}
	if !rep.TopFiles.Available || !rep.PerKey.Available || !rep.UnreadFiles.Available {
		t.Fatal("bagian lain harus tetap available")
	}
	if !strings.Contains(buf.String(), "section=zero_queries") {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestUsage_ListFilesFailSoft(t *testing.T) {
	r := &fakeRepo{}
	uc, _ := newUC(r, fakeFinder{found: true}, &fakeLister{err: errors.New("down")})
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid, Section: base.SectionUnreadFiles})
	if err != nil {
		t.Fatal(err)
	}
	if rep.UnreadFiles.Available || rep.UnreadFiles.ErrorCode != base.SectionUnavailable {
		t.Fatalf("%+v", rep.UnreadFiles)
	}
	if len(r.calls) != 0 {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestUsage_UnreadRepoFailSoft(t *testing.T) {
	r := &fakeRepo{unreadErr: errors.New("boom")}
	uc, _ := newUC(r, fakeFinder{found: true}, &fakeLister{files: []base.KnownFile{{Path: "a.md"}}})
	rep, _ := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid, Section: base.SectionUnreadFiles})
	if rep.UnreadFiles.Available || rep.UnreadFiles.ErrorCode != base.SectionUnavailable {
		t.Fatalf("%+v", rep.UnreadFiles)
	}
}

func TestUsage_SectionOnly(t *testing.T) {
	r := &fakeRepo{}
	uc, _ := newUC(r, fakeFinder{}, &fakeLister{})
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, Section: base.SectionKeys})
	if err != nil {
		t.Fatal(err)
	}
	if rep.PerKey == nil || rep.TopFiles != nil || rep.UnreadFiles != nil || rep.ZeroQueries != nil {
		t.Fatalf("%+v", rep)
	}
	if strings.Join(r.calls, ",") != "keys" {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestUsage_UnreadPassesPathsAndLimit(t *testing.T) {
	ts := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	r := &fakeRepo{unreadOut: []string{"b.md"}}
	l := &fakeLister{files: []base.KnownFile{{Path: "a.md", UpdatedAt: ts}, {Path: "b.md", UpdatedAt: ts}}, capped: true}
	uc, _ := newUC(r, fakeFinder{found: true}, l)
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid, Section: base.SectionUnreadFiles})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.unreadPaths, ",") != "a.md,b.md" || r.unreadLimit != base.UnreadLimit+1 {
		t.Fatalf("paths=%v limit=%d", r.unreadPaths, r.unreadLimit)
	}
	u := rep.UnreadFiles
	if !u.Available || len(u.Items) != 1 || u.Items[0].Path != "b.md" || !u.Truncated {
		t.Fatalf("%+v", u)
	}
}

func TestUsage_WindowFromNow(t *testing.T) {
	r := &fakeRepo{}
	uc, _ := newUC(r, fakeFinder{found: true}, &fakeLister{})
	rep, err := uc.Usage(context.Background(), base.UsageQuery{Range: base.Range7d, ProjectID: pid, Section: base.SectionTopFiles})
	if err != nil {
		t.Fatal(err)
	}
	if !r.w.From.Equal(fixedNow.Add(-168*time.Hour)) || !rep.From.Equal(r.w.From) {
		t.Fatalf("from %v", r.w.From)
	}
}
