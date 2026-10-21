package createfile

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

type fakeAccess struct {
	err error
}

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	if f.err != nil {
		return base.Actor{}, f.err
	}
	return actor, nil
}

type fakeRepo struct {
	calls                     []string
	inTx                      bool
	count                     int
	lockErr, countErr, insErr error
	inserts                   int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Repository) error) error {
	f.inTx = true
	defer func() { f.inTx = false }()
	return fn(f)
}
func (f *fakeRepo) LockProject(context.Context, string) error {
	f.calls = append(f.calls, "lock")
	return f.lockErr
}
func (f *fakeRepo) CountFiles(context.Context, string) (int, error) {
	f.calls = append(f.calls, "count")
	return f.count, f.countErr
}
func (f *fakeRepo) Insert(_ context.Context, file base.File) (base.File, error) {
	f.calls = append(f.calls, "insert")
	f.inserts++
	if f.insErr != nil {
		return base.File{}, f.insErr
	}
	return file, nil
}

type fakeAudit struct {
	events    []audit.Event
	repo      *fakeRepo
	inTxCalls int
}

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) {
	if f.repo != nil && f.repo.inTx {
		f.inTxCalls++
	}
	f.events = append(f.events, ev)
}

const maxFile = 100

func setup() (*Usecase, *fakeRepo, *fakeAudit) {
	r := &fakeRepo{}
	a := &fakeAudit{repo: r}
	return NewUsecase(r, fakeAccess{}, a, base.Limits{MaxFileBytes: maxFile, MaxProjectFiles: 3}), r, a
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

func TestSuccess(t *testing.T) {
	u, r, a := setup()
	meta := base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}
	content := "# Login\n\nAlur"
	dto, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "docs/auth/login.md", Content: content}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if dto.Path != "docs/auth/login.md" || dto.Title != "Login" || dto.SizeBytes != len(content) || len(dto.Version) != 64 ||
		dto.CreatedBy.Label != "Budi" || dto.UpdatedBy.Label != "Budi" {
		t.Fatalf("%+v", dto)
	}
	if strings.Join(r.calls, ",") != "lock,count,insert" {
		t.Fatal(r.calls)
	}
	if len(a.events) != 1 {
		t.Fatal(a.events)
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeFileCreated || ev.TargetType != "file" || ev.Target != dto.Path ||
		len(ev.Args) != 0 || ev.Result["bytes"] != len(content) || ev.Meta != meta || ev.Actor != actor {
		t.Fatalf("%+v", ev)
	}
	if a.inTxCalls != 0 {
		t.Fatal("audit dipanggil di dalam tx")
	}
}

func TestAuditNoContent(t *testing.T) {
	u, _, a := setup()
	secret := "RAHASIA-UNIK-XYZ"
	if _, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md", Content: "# " + secret}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	ev := a.events[0]
	if strings.Contains(fmt.Sprint(ev.Args, ev.Result), secret) {
		t.Fatal("isi bocor ke audit")
	}
}

func TestAccessErrors(t *testing.T) {
	for _, ae := range []error{base.ErrProjectNotFound, base.ErrUnauthenticated, errors.New("boom")} {
		r := &fakeRepo{}
		a := &fakeAudit{}
		u := NewUsecase(r, fakeAccess{err: ae}, a, base.Limits{MaxFileBytes: maxFile, MaxProjectFiles: 3})
		// path tidak valid: akses harus dievaluasi lebih dulu
		_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "../x"}, base.RequestMeta{})
		if !errors.Is(err, ae) {
			t.Fatalf("want %v got %v", ae, err)
		}
		if len(r.calls) != 0 || len(a.events) != 0 {
			t.Fatal("repo/audit tersentuh")
		}
	}
}

func TestInvalidPaths(t *testing.T) {
	cases := []struct{ path, reason string }{
		{"", "path_empty"}, {"/a.md", "path_absolute"}, {"../x.md", "path_traversal"}, {"a/./b.md", "path_traversal"},
		{"a//b.md", "path_empty_segment"}, {"a/", "path_empty_segment"}, {"a\x00b.md", "path_control_char"},
		{strings.Repeat("a/", 256) + "b.md", "path_too_long"}, {strings.Repeat("a", 256) + ".md", "segment_too_long"},
		{"\xff.md", "invalid_encoding"},
		{".git/config.md", "hidden"}, {"docs/.hidden/x.md", "hidden"}, {".env.md", "hidden"}, {".DS_Store", "hidden"}, {".env", "hidden"},
		{"a.yaml", "unsupported_type"}, {"README", "unsupported_type"}, {"x.md.bak", "unsupported_type"}, {"img.png", "unsupported_type"},
	}
	for _, c := range cases {
		u, r, a := setup()
		_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: c.path}, base.RequestMeta{})
		code, rs := reason(t, err)
		if code != errs.CodeInvalid || fmt.Sprint(rs) != c.reason {
			t.Errorf("%q: %v %v want %s", c.path, code, rs, c.reason)
		}
		if !errors.Is(err, errs.ErrInvalid) || len(r.calls) != 0 || len(a.events) != 0 {
			t.Errorf("%q: efek samping", c.path)
		}
	}
}

func TestValidPaths(t *testing.T) {
	for _, p := range []string{"docs/a.b.md", "A.MD", "a.markdown", "a.txt", "Docs/A.md"} {
		u, _, _ := setup()
		dto, err := u.Execute(context.Background(), "p1", CreateRequest{Path: p}, base.RequestMeta{})
		if err != nil || dto.Path != p {
			t.Errorf("%s: %v %v", p, dto.Path, err)
		}
	}
}

func TestContent(t *testing.T) {
	u, _, a := setup()
	ok := strings.Repeat("a", maxFile)
	if dto, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md", Content: ok}, base.RequestMeta{}); err != nil || dto.SizeBytes != maxFile {
		t.Fatal(dto.SizeBytes, err)
	}
	_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "b.md", Content: ok + "a"}, base.RequestMeta{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodePayloadTooLarge {
		t.Fatal(err)
	}
	if d := e.Details.(map[string]any); d["reason"] != base.ReasonTooLarge || d["max_bytes"] != int64(maxFile) {
		t.Fatal(d)
	}
	// BOM + tepat batas
	if dto, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "c.md", Content: "\ufeff" + ok}, base.RequestMeta{}); err != nil || dto.SizeBytes != maxFile {
		t.Fatal(dto.SizeBytes, err)
	}
	for _, bad := range []string{"a\x00b", "\xff\xfe"} {
		_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "d.md", Content: bad}, base.RequestMeta{})
		if c, rs := reason(t, err); c != errs.CodeInvalid || rs != base.ReasonInvalidEncoding {
			t.Fatal(c, rs)
		}
	}
	dto, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "e.md"}, base.RequestMeta{})
	if err != nil || dto.SizeBytes != 0 || dto.Title != "" {
		t.Fatalf("%+v %v", dto, err)
	}
	if len(a.events) != 3 {
		t.Fatalf("audit %d", len(a.events))
	}
}

func TestQuota(t *testing.T) {
	u, r, a := setup()
	r.count = 3
	_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md"}, base.RequestMeta{})
	if c, rs := reason(t, err); c != errs.CodeInvalid || rs != base.ReasonProjectFileLimit {
		t.Fatal(c, rs)
	}
	if r.inserts != 0 || len(a.events) != 0 {
		t.Fatal("insert/audit tidak boleh")
	}
	r.count = 2
	if _, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicate(t *testing.T) {
	u, r, a := setup()
	r.insErr = base.ErrPathExists
	_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md"}, base.RequestMeta{})
	if c, rs := reason(t, err); c != errs.CodeConflict || rs != base.ReasonPathExists {
		t.Fatal(c, rs)
	}
	if len(a.events) != 0 {
		t.Fatal("audit")
	}
}

func TestInfraErrors(t *testing.T) {
	boom := errors.New("boom")
	for _, set := range []func(*fakeRepo){
		func(r *fakeRepo) { r.lockErr = boom },
		func(r *fakeRepo) { r.countErr = boom },
		func(r *fakeRepo) { r.insErr = boom },
	} {
		u, r, a := setup()
		set(r)
		_, err := u.Execute(context.Background(), "p1", CreateRequest{Path: "a.md"}, base.RequestMeta{})
		var e *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &e) || len(a.events) != 0 {
			t.Fatalf("%v", err)
		}
	}
}
