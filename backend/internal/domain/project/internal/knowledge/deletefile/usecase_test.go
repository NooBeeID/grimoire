package deletefile

import (
	"context"
	"errors"
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
	size    int
	found   bool
	err     error
	calls   int
	pid, pt string
}

func (f *fakeRepo) DeleteByPath(_ context.Context, pid, path string) (int, bool, error) {
	f.calls++
	f.pid, f.pt = pid, path
	return f.size, f.found, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

func run(acc error, r *fakeRepo, path string) (error, *fakeAudit) {
	a := &fakeAudit{}
	meta := base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}
	return NewUsecase(fakeAccess{err: acc}, r, a).Execute(context.Background(), params{ProjectID: "p1", Path: path}, meta), a
}

func TestSuccess(t *testing.T) {
	r := &fakeRepo{size: 1234, found: true}
	err, a := run(nil, r, "docs/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if r.pid != "p1" || r.pt != "docs/a.md" || len(a.events) != 1 {
		t.Fatalf("%+v %d", r, len(a.events))
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeFileDeleted || ev.TargetType != "file" || ev.Target != "docs/a.md" ||
		ev.Actor != actor || ev.ProjectID != "p1" || ev.Meta.IP != "1.1.1.1" ||
		len(ev.Args) != 0 || len(ev.Result) != 1 || ev.Result["bytes"] != 1234 {
		t.Fatalf("%+v", ev)
	}
}

func TestNotFound(t *testing.T) {
	err, a := run(nil, &fakeRepo{}, "a.md")
	if !errors.Is(err, base.ErrFileNotFound) || len(a.events) != 0 {
		t.Fatalf("%v %d", err, len(a.events))
	}
}

func TestAccessErrors(t *testing.T) {
	infra := errors.New("bus down")
	for name, tc := range map[string]struct {
		acc  error
		path string
	}{
		"non-member":      {base.ErrProjectNotFound, "a.md"},
		"access-first":    {base.ErrProjectNotFound, ""},
		"unauthenticated": {base.ErrUnauthenticated, "a.md"},
		"infra":           {infra, "a.md"},
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRepo{found: true}
			err, a := run(tc.acc, r, tc.path)
			if err != tc.acc || r.calls != 0 || len(a.events) != 0 {
				t.Fatalf("%v %d %d", err, r.calls, len(a.events))
			}
		})
	}
}

func TestInvalidPath(t *testing.T) {
	for p, want := range map[string]base.Reason{
		"":                       base.ReasonPathEmpty,
		"/a.md":                  base.ReasonPathAbsolute,
		"../x.md":                base.ReasonPathTraversal,
		"a//b.md":                base.ReasonPathEmptySegment,
		"a\x00.md":               base.ReasonPathControlChar,
		strings.Repeat("a", 513): base.ReasonPathTooLong,
	} {
		r := &fakeRepo{found: true}
		err, a := run(nil, r, p)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid || !errors.Is(err, errs.ErrInvalid) ||
			e.Details.(map[string]any)["reason"] != want || r.calls != 0 || len(a.events) != 0 {
			t.Fatalf("%q: %v", p, err)
		}
	}
}

func TestRepoError(t *testing.T) {
	boom := errors.New("db")
	err, a := run(nil, &fakeRepo{err: boom}, "a.md")
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "deletefile:") || len(a.events) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestPathUsedAsIs(t *testing.T) {
	r := &fakeRepo{found: true}
	if err, _ := run(nil, r, "Docs/A.MD"); err != nil || r.pt != "Docs/A.MD" {
		t.Fatalf("%v %q", err, r.pt)
	}
}
