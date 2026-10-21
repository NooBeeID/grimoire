package updatefile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

// fakeRepo memodelkan satu baris di memori.
type fakeRepo struct {
	row        *base.File
	updates    int
	findErr    error
	updErr     error
	forceMiss  int // jumlah panggilan UpdateIfVersion pertama yang dipaksa found=false
	finds      int
	updateSeen bool
}

func (f *fakeRepo) FindByPath(context.Context, string, string) (base.File, bool, error) {
	f.finds++
	if f.findErr != nil {
		return base.File{}, false, f.findErr
	}
	if f.row == nil {
		return base.File{}, false, nil
	}
	return *f.row, true, nil
}

func (f *fakeRepo) UpdateIfVersion(_ context.Context, _, _ string, exp []byte, n base.File) (base.File, int, bool, error) {
	f.updates++
	if f.updErr != nil {
		return base.File{}, 0, false, f.updErr
	}
	if f.forceMiss > 0 {
		f.forceMiss--
		return base.File{}, 0, false, nil
	}
	if f.row == nil || !bytes.Equal(f.row.Hash, exp) {
		return base.File{}, 0, false, nil
	}
	before := f.row.SizeBytes
	f.row.Title, f.row.Content, f.row.Hash, f.row.SizeBytes = n.Title, n.Content, n.Hash, n.SizeBytes
	f.row.UpdatedBy, f.row.UpdatedByLabel = n.UpdatedBy, n.UpdatedByLabel
	f.row.UpdatedAt = time.Now().UTC()
	return *f.row, before, true, nil
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

const maxFile = 100

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func seed(content string) *base.File {
	f := base.NewFile("p1", "docs/a.md", content, base.Actor{ID: "u1", Label: "Budi"})
	f.CreatedAt, f.UpdatedAt = t0, t0
	return &f
}

func setup(content string) (*Usecase, *fakeRepo, *fakeAudit) {
	r := &fakeRepo{row: seed(content)}
	a := &fakeAudit{}
	return NewUsecase(r, fakeAccess{}, a, base.Limits{MaxFileBytes: maxFile}), r, a
}

func ver(content string) string { return base.Version(base.Hash(content)) }

func sp(s string) *string { return &s }

func in(content *string, version string) Input {
	return Input{ProjectID: "p1", Path: "docs/a.md", Content: content, Version: version}
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
	u, r, a := setup("# A\nlama")
	meta := base.RequestMeta{IP: "1.1.1.1", UserAgent: "ua"}
	i := in(sp("# B\nbaru!"), ver("# A\nlama"))
	i.Meta = meta
	dto, err := u.Execute(context.Background(), i)
	if err != nil {
		t.Fatal(err)
	}
	if dto.Content != "# B\nbaru!" || dto.Title != "B" || dto.Version != ver("# B\nbaru!") || dto.SizeBytes != 9 ||
		dto.UpdatedBy.Label != "Sari" || dto.CreatedBy.Label != "Budi" {
		t.Fatalf("%+v", dto)
	}
	if len(a.events) != 1 {
		t.Fatal(a.events)
	}
	ev := a.events[0]
	if ev.Action != record.ActionKnowledgeFileUpdated || ev.TargetType != "file" || ev.Target != "docs/a.md" ||
		len(ev.Args) != 0 || ev.Result["bytes_before"] != 8 || ev.Result["bytes"] != 9 || ev.Meta != meta || ev.Actor != actor {
		t.Fatalf("%+v", ev)
	}
	for _, v := range ev.Result {
		if s, ok := v.(string); ok && strings.Contains(s, "baru") {
			t.Fatal("isi bocor ke audit")
		}
	}
	if r.updates != 1 {
		t.Fatal(r.updates)
	}
}

func TestNoop(t *testing.T) {
	u, r, a := setup("sama")
	dto, err := u.Execute(context.Background(), in(sp("sama"), ver("sama")))
	if err != nil || dto.Content != "sama" || !dto.UpdatedAt.Equal(t0) {
		t.Fatalf("%+v %v", dto, err)
	}
	if r.updates != 0 || len(a.events) != 0 {
		t.Fatal("no-op tidak boleh menulis/audit")
	}
}

func TestNoopFalse(t *testing.T) {
	u, r, a := setup("punya orang lain")
	_, err := u.Execute(context.Background(), in(sp("lama"), ver("lama")))
	code, rs := reason(t, err)
	if code != errs.CodeConflict || rs != base.ReasonVersionConflict {
		t.Fatalf("%v %v", code, rs)
	}
	var e *errs.Error
	errors.As(err, &e)
	cur := e.Details.(map[string]any)["current"].(base.ConflictCurrent)
	if cur.Content != "punya orang lain" || r.updates != 0 || len(a.events) != 0 {
		t.Fatalf("%+v", cur)
	}
}

func TestNoopMissingFile(t *testing.T) {
	u, r, _ := setup("x")
	r.row = nil
	if _, err := u.Execute(context.Background(), in(sp("x"), ver("x"))); !errors.Is(err, base.ErrFileNotFound) {
		t.Fatal(err)
	}
}

func TestConflictAC3(t *testing.T) {
	u, r, a := setup("v0")
	if _, err := u.Execute(context.Background(), in(sp("isi A"), ver("v0"))); err != nil {
		t.Fatal(err)
	}
	_, err := u.Execute(context.Background(), in(sp("isi B"), ver("v0")))
	code, rs := reason(t, err)
	if code != errs.CodeConflict || rs != base.ReasonVersionConflict {
		t.Fatalf("%v %v", code, rs)
	}
	var e *errs.Error
	errors.As(err, &e)
	cur := e.Details.(map[string]any)["current"].(base.ConflictCurrent)
	if cur.Content != "isi A" || cur.Version != ver("isi A") || cur.UpdatedByLabel != "Sari" {
		t.Fatalf("%+v", cur)
	}
	if r.row.Content != "isi A" || len(a.events) != 1 {
		t.Fatal("isi harus tetap A, audit 1")
	}
}

func TestMissingFile(t *testing.T) {
	u, r, a := setup("x")
	r.row = nil
	_, err := u.Execute(context.Background(), in(sp("y"), ver("x")))
	if !errors.Is(err, base.ErrFileNotFound) || len(a.events) != 0 {
		t.Fatal(err)
	}
}

func TestInvalidVersion(t *testing.T) {
	for _, v := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("zz", 32)} {
		u, r, _ := setup("x")
		_, err := u.Execute(context.Background(), in(sp("y"), v))
		if code, rs := reason(t, err); code != errs.CodeInvalid || rs != base.ReasonInvalidVersion || r.updates+r.finds != 0 {
			t.Errorf("%q: %v %v", v, code, rs)
		}
	}
}

func TestSizeLimit(t *testing.T) {
	u, _, _ := setup("x")
	_, err := u.Execute(context.Background(), in(sp(strings.Repeat("a", maxFile+1)), ver("x")))
	code, rs := reason(t, err)
	var e *errs.Error
	errors.As(err, &e)
	if code != errs.CodePayloadTooLarge || rs != base.ReasonTooLarge || e.Details.(map[string]any)["max_bytes"] != int64(maxFile) {
		t.Fatalf("%v %v %v", code, rs, e.Details)
	}
	if _, err := u.Execute(context.Background(), in(sp(strings.Repeat("a", maxFile)), ver("x"))); err != nil {
		t.Fatal(err)
	}
	u, _, _ = setup("x")
	if _, err := u.Execute(context.Background(), in(sp("\xef\xbb\xbf"+strings.Repeat("a", maxFile)), ver("x"))); err != nil {
		t.Fatalf("BOM + batas harus lolos: %v", err)
	}
}

func TestInvalidEncoding(t *testing.T) {
	for _, c := range []string{"\xff\xfe", "a\x00b"} {
		u, _, _ := setup("x")
		_, err := u.Execute(context.Background(), in(sp(c), ver("x")))
		if code, rs := reason(t, err); code != errs.CodeInvalid || rs != base.ReasonInvalidEncoding {
			t.Errorf("%q: %v %v", c, code, rs)
		}
	}
}

func TestContentNilAndEmpty(t *testing.T) {
	u, _, _ := setup("x")
	_, err := u.Execute(context.Background(), in(nil, ver("x")))
	var e *errs.Error
	if !errors.As(err, &e) || e.Message != "content wajib diisi (boleh string kosong)" {
		t.Fatal(err)
	}
	if code, rs := reason(t, err); code != errs.CodeInvalid || rs != base.ReasonInvalidEncoding {
		t.Fatal(code, rs)
	}
	dto, err := u.Execute(context.Background(), in(sp(""), ver("x")))
	if err != nil || dto.SizeBytes != 0 || dto.Title != "" {
		t.Fatalf("%+v %v", dto, err)
	}
}

func TestBOMStripped(t *testing.T) {
	u, _, _ := setup("x")
	dto, err := u.Execute(context.Background(), in(sp("\xef\xbb\xbf# T"), ver("x")))
	if err != nil || dto.Content != "# T" || dto.Version != ver("# T") || dto.Title != "T" {
		t.Fatalf("%+v %v", dto, err)
	}
}

func TestInvalidPath(t *testing.T) {
	cases := map[string]base.Reason{
		"": base.ReasonPathEmpty, "/a.md": base.ReasonPathAbsolute, "../x.md": base.ReasonPathTraversal,
		"a//b.md": base.ReasonPathEmptySegment, "a\x00.md": base.ReasonPathControlChar,
		strings.Repeat("a", 513): base.ReasonPathTooLong,
	}
	for p, want := range cases {
		u, r, _ := setup("x")
		i := in(sp("y"), ver("x"))
		i.Path = p
		_, err := u.Execute(context.Background(), i)
		if code, rs := reason(t, err); code != errs.CodeInvalid || rs != want || r.updates != 0 {
			t.Errorf("%q: %v %v", p, code, rs)
		}
	}
}

func TestValidationOrder(t *testing.T) {
	// access menang atas path salah
	r := &fakeRepo{row: seed("x")}
	u := NewUsecase(r, fakeAccess{err: base.ErrProjectNotFound}, &fakeAudit{}, base.Limits{MaxFileBytes: maxFile})
	i := in(sp("y"), "bad")
	i.Path = "../x"
	if _, err := u.Execute(context.Background(), i); !errors.Is(err, base.ErrProjectNotFound) || r.updates+r.finds != 0 {
		t.Fatal(err)
	}
	// path salah menang atas content salah
	u, _, _ = setup("x")
	i = in(sp("a\x00"), "bad")
	i.Path = "../x"
	_, err := u.Execute(context.Background(), i)
	if _, rs := reason(t, err); rs != base.ReasonPathTraversal {
		t.Fatal(rs)
	}
	// isi 413 menang atas version salah
	_, err = u.Execute(context.Background(), in(sp(strings.Repeat("a", maxFile+1)), "bad"))
	if code, rs := reason(t, err); code != errs.CodePayloadTooLarge || rs != base.ReasonTooLarge {
		t.Fatal(code, rs)
	}
}

func TestAccessErrors(t *testing.T) {
	boom := errors.New("bus down")
	for _, e := range []error{base.ErrProjectNotFound, base.ErrUnauthenticated, boom} {
		r := &fakeRepo{row: seed("x")}
		u := NewUsecase(r, fakeAccess{err: e}, &fakeAudit{}, base.Limits{MaxFileBytes: maxFile})
		if _, err := u.Execute(context.Background(), in(sp("y"), ver("x"))); !errors.Is(err, e) || r.updates+r.finds != 0 {
			t.Errorf("%v: %v", e, err)
		}
	}
}

func TestDBError(t *testing.T) {
	u, r, a := setup("x")
	r.updErr = fmt.Errorf("db: %w", errors.New("down"))
	_, err := u.Execute(context.Background(), in(sp("y"), ver("x")))
	var e *errs.Error
	if err == nil || errors.As(err, &e) || len(a.events) != 0 {
		t.Fatal(err)
	}
}

func TestRetryOnce(t *testing.T) {
	u, r, a := setup("x")
	r.forceMiss = 1
	dto, err := u.Execute(context.Background(), in(sp("y"), ver("x")))
	if err != nil || dto.Content != "y" || r.updates != 2 || len(a.events) != 1 {
		t.Fatalf("%v %d %d", err, r.updates, len(a.events))
	}
	u, r, a = setup("x")
	r.forceMiss = 5
	_, err = u.Execute(context.Background(), in(sp("y"), ver("x")))
	if code, rs := reason(t, err); code != errs.CodeConflict || rs != base.ReasonVersionConflict || r.updates != 2 || len(a.events) != 0 {
		t.Fatalf("%v %v %d", code, rs, r.updates)
	}
}
