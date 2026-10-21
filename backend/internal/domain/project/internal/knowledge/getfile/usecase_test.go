package getfile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
)

type fakeAccess struct {
	err   error
	calls int
}

func (f *fakeAccess) Require(context.Context, string) (base.Actor, error) {
	f.calls++
	if f.err != nil {
		return base.Actor{}, f.err
	}
	return base.Actor{ID: "u1", Role: "developer", Label: "Budi"}, nil
}

type fakeFiles struct {
	file    base.File
	found   bool
	err     error
	calls   int
	project string
	path    string
}

func (f *fakeFiles) FindByPath(_ context.Context, projectID, path string) (base.File, bool, error) {
	f.calls++
	f.project, f.path = projectID, path
	return f.file, f.found, f.err
}

func mkFile(content string) base.File {
	t := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	f := base.NewFile("p1", "docs/login.md", content, base.Actor{ID: "u1", Label: "Budi"})
	f.CreatedAt, f.UpdatedAt = t, t
	return f
}

func code(t *testing.T, err error) errs.Code {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e.Code
}

func TestExecute_Success(t *testing.T) {
	content := "# Login\n"
	ff := &fakeFiles{file: mkFile(content), found: true}
	dto, err := NewUsecase(&fakeAccess{}, ff).Execute(context.Background(), "p1", "docs/login.md")
	if err != nil {
		t.Fatal(err)
	}
	if dto.Title != "Login" || dto.Content != content || dto.SizeBytes != len(content) {
		t.Fatalf("dto=%+v", dto)
	}
	if dto.Version != base.Version(base.Hash(content)) || len(dto.Version) != 64 || dto.Version != strings.ToLower(dto.Version) {
		t.Fatalf("version=%s", dto.Version)
	}
	if dto.CreatedBy.ID != "u1" || dto.UpdatedBy.Label != "Budi" {
		t.Fatalf("refs=%+v %+v", dto.CreatedBy, dto.UpdatedBy)
	}
}

func TestExecute_VersionFollowsContent(t *testing.T) {
	get := func(c string) string {
		d, err := NewUsecase(&fakeAccess{}, &fakeFiles{file: mkFile(c), found: true}).Execute(context.Background(), "p1", "a.md")
		if err != nil {
			t.Fatal(err)
		}
		return d.Version
	}
	if get("x") != get("x") || get("x") == get("y") {
		t.Fatal("version tidak mengikuti isi")
	}
}

func TestExecute_NotFound(t *testing.T) {
	ff := &fakeFiles{}
	_, err := NewUsecase(&fakeAccess{}, ff).Execute(context.Background(), "p1", "docs/x.md")
	if !errors.Is(err, base.ErrFileNotFound) || code(t, err) != errs.CodeNotFound {
		t.Fatalf("err=%v", err)
	}
	if ff.project != "p1" || ff.path != "docs/x.md" {
		t.Fatalf("finder args %q %q", ff.project, ff.path)
	}
}

func TestExecute_AccessErrors(t *testing.T) {
	boom := errors.New("bus down")
	cases := []struct {
		name string
		err  error
		path string
		want error
	}{
		{"non-anggota", base.ErrProjectNotFound, "docs/a.md", base.ErrProjectNotFound},
		{"non-anggota path jelek", base.ErrProjectNotFound, "../x", base.ErrProjectNotFound},
		{"unauth", base.ErrUnauthenticated, "docs/a.md", base.ErrUnauthenticated},
		{"bus gagal", boom, "docs/a.md", boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ff := &fakeFiles{}
			_, err := NewUsecase(&fakeAccess{err: c.err}, ff).Execute(context.Background(), "p1", c.path)
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v", err)
			}
			if ff.calls != 0 {
				t.Fatal("finder tidak boleh dipanggil")
			}
		})
	}
}

func TestExecute_InvalidPath(t *testing.T) {
	cases := map[string]string{
		"":                       "path_empty",
		"/a.md":                  "path_absolute",
		"../x.md":                "path_traversal",
		"a/./b.md":               "path_traversal",
		"a//b.md":                "path_empty_segment",
		"a\x00b.md":              "path_control_char",
		strings.Repeat("a", 513): "path_too_long",
	}
	for p, reason := range cases {
		ff := &fakeFiles{}
		_, err := NewUsecase(&fakeAccess{}, ff).Execute(context.Background(), "p1", p)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%q: err=%v", p, err)
		}
		if !strings.Contains(fmtDetails(e.Details), reason) {
			t.Fatalf("%q: details=%v want %s", p, e.Details, reason)
		}
		if ff.calls != 0 {
			t.Fatalf("%q: finder dipanggil", p)
		}
	}
}

func fmtDetails(d any) string { return strings.TrimSpace(fmtSprint(d)) }

func TestExecute_DBError(t *testing.T) {
	boom := errors.New("db")
	_, err := NewUsecase(&fakeAccess{}, &fakeFiles{err: boom}).Execute(context.Background(), "p1", "a.md")
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "getfile: find:") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_CaseSensitivePath(t *testing.T) {
	ff := &fakeFiles{file: mkFile("x"), found: true}
	if _, err := NewUsecase(&fakeAccess{}, ff).Execute(context.Background(), "p1", "Docs/A.md"); err != nil {
		t.Fatal(err)
	}
	if ff.path != "Docs/A.md" {
		t.Fatalf("path=%q", ff.path)
	}
}
