package movefile

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

var actor = base.Actor{ID: "u2", Role: "developer", Label: "Sari"}

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	if f.err != nil {
		return base.Actor{}, f.err
	}
	return actor, nil
}

type fakeRepo struct {
	calls      []string
	exists     map[string]bool
	renameN    int64
	renameErr  error
	existsErr  error
	file       base.File
	found      bool
	renameFrom string
	renameTo   string
	renameAct  base.Actor
	inTxDone   bool
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Repository) error) error {
	err := fn(f)
	f.inTxDone = true
	return err
}
func (f *fakeRepo) LockProject(context.Context, string) error {
	f.calls = append(f.calls, "lock")
	return nil
}
func (f *fakeRepo) PathExists(_ context.Context, _, p string) (bool, error) {
	f.calls = append(f.calls, "exists:"+p)
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists[p], nil
}
func (f *fakeRepo) Rename(_ context.Context, _, from, to string, a base.Actor) (int64, error) {
	f.calls = append(f.calls, "rename")
	f.renameFrom, f.renameTo, f.renameAct = from, to, a
	return f.renameN, f.renameErr
}
func (f *fakeRepo) FindByPath(_ context.Context, _, p string) (base.File, bool, error) {
	f.calls = append(f.calls, "find")
	return f.file, f.found, nil
}

type fakeAudit struct {
	events []audit.Event
	repo   *fakeRepo
	inTx   []bool // apakah InTx sudah selesai saat Record
}

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) {
	f.events = append(f.events, ev)
	f.inTx = append(f.inTx, f.repo.inTxDone)
}

func setup(from, to string) (*Usecase, *fakeRepo, *fakeAudit) {
	file := base.NewFile("p1", to, "# Todo\nisi", base.Actor{ID: "u1", Label: "Budi"})
	r := &fakeRepo{exists: map[string]bool{from: true}, renameN: 1, file: file, found: true}
	a := &fakeAudit{repo: r}
	return NewUsecase(fakeAccess{}, r, a), r, a
}

func run(u *Usecase, from, to string) (base.FileDTO, error) {
	return u.Execute(context.Background(), "p1", Request{From: from, To: to}, base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"})
}

func reason(t *testing.T, err error) (errs.Code, any) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	d, _ := e.Details.(map[string]any)
	return e.Code, d["reason"]
}

func TestSuccessRenameAndMove(t *testing.T) {
	for _, c := range [][2]string{{"docs/a.md", "docs/b.md"}, {"notes/todo.md", "archive/todo.md"}, {"Docs/A.md", "docs/a.md"}, {"a.md", "docs/A.MD"}} {
		u, r, a := setup(c[0], c[1])
		dto, err := run(u, c[0], c[1])
		if err != nil {
			t.Fatal(err)
		}
		if dto.Path != c[1] || len(dto.Version) != 64 || dto.Title != "Todo" || dto.CreatedBy.Label != "Budi" {
			t.Fatalf("%+v", dto)
		}
		if got := strings.Join(r.calls, ","); got != "lock,exists:"+c[0]+",exists:"+c[1]+",rename,find" {
			t.Fatal(got)
		}
		if r.renameFrom != c[0] || r.renameTo != c[1] || r.renameAct != actor {
			t.Fatalf("%+v", r)
		}
		if len(a.events) != 1 {
			t.Fatal(a.events)
		}
	}
}

func TestAuditContentAndAfterCommit(t *testing.T) {
	u, _, a := setup("notes/todo.md", "archive/todo.md")
	meta := base.RequestMeta{IP: "9.9.9.9", UserAgent: "ua"}
	dto, err := u.Execute(context.Background(), "p1", Request{From: "notes/todo.md", To: "archive/todo.md"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeFileMoved || ev.TargetType != "file" || ev.Target != "archive/todo.md" ||
		ev.Args["from"] != "notes/todo.md" || ev.Args["to"] != "archive/todo.md" || ev.Result["bytes"] != dto.SizeBytes ||
		ev.Meta != meta || ev.Actor != actor || ev.ProjectID != "p1" {
		t.Fatalf("%+v", ev)
	}
	if !a.inTx[0] {
		t.Fatal("audit harus setelah InTx")
	}
	for _, m := range []map[string]any{ev.Args, ev.Result} {
		for _, v := range m {
			if s, ok := v.(string); ok && strings.Contains(s, "isi") {
				t.Fatal("isi bocor")
			}
		}
	}
}

func TestTargetExists(t *testing.T) {
	u, r, a := setup("a.md", "b.md")
	r.exists["b.md"] = true
	_, err := run(u, "a.md", "b.md")
	code, rs := reason(t, err)
	if !errors.Is(err, base.ErrPathExists) || code != errs.CodeConflict || rs != base.ReasonPathExists {
		t.Fatalf("%v %v %v", err, code, rs)
	}
	if strings.Contains(strings.Join(r.calls, ","), "rename") || len(a.events) != 0 {
		t.Fatal(r.calls)
	}
}

func TestSourceMissing(t *testing.T) {
	for _, toExists := range []bool{false, true} {
		u, r, a := setup("a.md", "b.md")
		r.exists = map[string]bool{"b.md": toExists}
		_, err := run(u, "a.md", "b.md")
		if !errors.Is(err, base.ErrFileNotFound) {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(r.calls, ","), "exists:b.md") || len(a.events) != 0 {
			t.Fatal(r.calls)
		}
	}
}

func TestRenameZeroRows(t *testing.T) {
	u, r, a := setup("a.md", "b.md")
	r.renameN = 0
	if _, err := run(u, "a.md", "b.md"); !errors.Is(err, base.ErrFileNotFound) || len(a.events) != 0 {
		t.Fatal(err)
	}
}

func TestRenameUniqueViolation(t *testing.T) {
	u, r, a := setup("a.md", "b.md")
	r.renameErr = base.ErrPathExists
	_, err := run(u, "a.md", "b.md")
	code, _ := reason(t, err)
	if code != errs.CodeConflict || len(a.events) != 0 {
		t.Fatal(err)
	}
}

func TestAccessErrors(t *testing.T) {
	for _, e := range []error{base.ErrProjectNotFound, base.ErrUnauthenticated, errors.New("bus down")} {
		r := &fakeRepo{}
		a := &fakeAudit{repo: r}
		u := NewUsecase(fakeAccess{err: e}, r, a)
		_, err := run(u, "../x", "")
		if !errors.Is(err, e) || len(r.calls) != 0 || len(a.events) != 0 {
			t.Fatalf("%v %v", err, r.calls)
		}
	}
}

func TestValidation(t *testing.T) {
	long := strings.Repeat("a", 513)
	cases := []struct {
		name, from, to string
		want           base.Reason
	}{
		{"from empty", "", "b.md", base.ReasonPathEmpty},
		{"from abs", "/a.md", "b.md", base.ReasonPathAbsolute},
		{"from traversal", "../x.md", "b.md", base.ReasonPathTraversal},
		{"from dot", "a/./b.md", "b.md", base.ReasonPathTraversal},
		{"from empty seg", "a//b.md", "b.md", base.ReasonPathEmptySegment},
		{"from ctrl", "a\x00b.md", "b.md", base.ReasonPathControlChar},
		{"from long", long, "b.md", base.ReasonPathTooLong},
		{"to empty", "a.md", "", base.ReasonPathEmpty},
		{"to abs", "a.md", "/b.md", base.ReasonPathAbsolute},
		{"to traversal", "a.md", "../x.md", base.ReasonPathTraversal},
		{"to long", "a.md", long, base.ReasonPathTooLong},
		{"hidden git", "a.md", ".git/config.md", base.ReasonHidden},
		{"hidden dir", "a.md", "docs/.draft/x.md", base.ReasonHidden},
		{"hidden file", "a.md", ".env.md", base.ReasonHidden},
		{"yaml", "a.md", "docs/a.yaml", base.ReasonUnsupportedType},
		{"no ext", "a.md", "README", base.ReasonUnsupportedType},
		{"bak", "a.md", "x.md.bak", base.ReasonUnsupportedType},
		{"same", "a.md", "a.md", base.ReasonSamePath},
		{"order from first", "../x", ".git/x.md", base.ReasonPathTraversal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, r, a := setup(c.from, c.to)
			_, err := run(u, c.from, c.to)
			code, rs := reason(t, err)
			if code != errs.CodeInvalid || rs != c.want {
				t.Fatalf("%v %v", code, rs)
			}
			if len(r.calls) != 0 || len(a.events) != 0 {
				t.Fatal(r.calls)
			}
		})
	}
}

func TestInfraErrorsWrapped(t *testing.T) {
	boom := fmt.Errorf("boom")
	for _, mode := range []string{"exists", "rename"} {
		u, r, a := setup("a.md", "b.md")
		if mode == "exists" {
			r.existsErr = boom
		} else {
			r.renameErr = boom
		}
		_, err := run(u, "a.md", "b.md")
		var e *errs.Error
		if err == nil || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "movefile: ") || len(a.events) != 0 {
			t.Fatalf("%v", err)
		}
	}
}

func TestReloadMissing(t *testing.T) {
	u, r, a := setup("a.md", "b.md")
	r.found = false
	_, err := run(u, "a.md", "b.md")
	var e *errs.Error
	if err == nil || errors.As(err, &e) || len(a.events) != 0 {
		t.Fatal(err)
	}
}
