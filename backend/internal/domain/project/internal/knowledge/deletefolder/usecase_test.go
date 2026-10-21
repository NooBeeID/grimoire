package deletefolder

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
	n     int
	err   error
	calls int
	pid   string
	like  string
}

func (f *fakeRepo) DeleteByPrefix(_ context.Context, pid, like string) (int, error) {
	f.calls++
	f.pid, f.like = pid, like
	return f.n, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

func run(acc error, r *fakeRepo, path string) (Output, error, *fakeAudit) {
	a := &fakeAudit{}
	meta := base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}
	out, err := NewUsecase(fakeAccess{err: acc}, r, a).Execute(context.Background(), params{ProjectID: "p1", Path: path}, meta)
	return out, err, a
}

func reasonOf(err error) any {
	var e *errs.Error
	if errors.As(err, &e) {
		if m, ok := e.Details.(map[string]any); ok {
			return m["reason"]
		}
	}
	return nil
}

func TestSuccess(t *testing.T) {
	r := &fakeRepo{n: 3}
	out, err, a := run(nil, r, "docs")
	if err != nil || out.Deleted != 3 {
		t.Fatalf("%v %+v", err, out)
	}
	if r.pid != "p1" || r.like != "docs/%" || len(a.events) != 1 {
		t.Fatalf("%+v %d", r, len(a.events))
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeFolderDeleted || ev.TargetType != "folder" || ev.Target != "docs" ||
		ev.Actor != actor || ev.ProjectID != "p1" || ev.Meta.IP != "1.1.1.1" ||
		len(ev.Args) != 0 || len(ev.Result) != 1 || ev.Result["count"] != 3 {
		t.Fatalf("%+v", ev)
	}
}

func TestTrailingSlashSameAsPlain(t *testing.T) {
	for _, p := range []string{"docs", "docs/"} {
		r := &fakeRepo{n: 1}
		_, err, a := run(nil, r, p)
		if err != nil || r.like != "docs/%" || a.events[0].Target != "docs" {
			t.Fatalf("%q: %v %q", p, err, r.like)
		}
	}
}

func TestLikeEscaped(t *testing.T) {
	r := &fakeRepo{n: 1}
	if _, err, _ := run(nil, r, `a_b%c\d`); err != nil || r.like != `a\_b\%c\\d/%` {
		t.Fatalf("%v %q", err, r.like)
	}
}

func TestAccessErrors(t *testing.T) {
	infra := errors.New("bus down")
	for name, tc := range map[string]struct {
		acc  error
		path string
	}{
		"non-member":      {base.ErrProjectNotFound, "docs"},
		"access-first":    {base.ErrProjectNotFound, "../x"},
		"access-empty":    {base.ErrProjectNotFound, ""},
		"unauthenticated": {base.ErrUnauthenticated, "docs"},
		"infra":           {infra, "docs"},
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRepo{n: 1}
			_, err, a := run(tc.acc, r, tc.path)
			if err != tc.acc || r.calls != 0 || len(a.events) != 0 {
				t.Fatalf("%v %d %d", err, r.calls, len(a.events))
			}
		})
	}
}

func TestRootRejected(t *testing.T) {
	for p, want := range map[string]base.Reason{
		"":  base.ReasonPrefixRequired,
		"/": base.ReasonPathAbsolute,
	} {
		r := &fakeRepo{n: 1}
		_, err, a := run(nil, r, p)
		if !errors.Is(err, errs.ErrInvalid) || reasonOf(err) != want || r.calls != 0 || len(a.events) != 0 {
			t.Fatalf("%q: %v", p, err)
		}
	}
}

func TestInvalidPath(t *testing.T) {
	for p, want := range map[string]base.Reason{
		"../x":                   base.ReasonPathTraversal,
		"/docs":                  base.ReasonPathAbsolute,
		"docs//a":                base.ReasonPathEmptySegment,
		"a/./b":                  base.ReasonPathTraversal,
		"a\x00b":                 base.ReasonPathControlChar,
		strings.Repeat("a", 513): base.ReasonPathTooLong,
	} {
		r := &fakeRepo{n: 1}
		_, err, a := run(nil, r, p)
		if !errors.Is(err, errs.ErrInvalid) || reasonOf(err) != want || r.calls != 0 || len(a.events) != 0 {
			t.Fatalf("%q: %v", p, err)
		}
	}
}

func TestNotFound(t *testing.T) {
	_, err, a := run(nil, &fakeRepo{}, "docs")
	if !errors.Is(err, base.ErrFolderNotFound) || len(a.events) != 0 {
		t.Fatalf("%v %d", err, len(a.events))
	}
}

func TestRepoError(t *testing.T) {
	boom := errors.New("db")
	_, err, a := run(nil, &fakeRepo{err: boom}, "docs")
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "deletefolder: delete by prefix:") || len(a.events) != 0 {
		t.Fatalf("%v", err)
	}
}
