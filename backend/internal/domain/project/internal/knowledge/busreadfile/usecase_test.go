package busreadfile

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/knowledge/readfile"
)

const (
	pidA = "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
	pidB = "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e70"
)

type fakeFiles struct {
	data    map[string]map[string]string
	findErr error
	listErr error
	finds   int
	lists   int
	updated time.Time
}

func newFake() *fakeFiles { return &fakeFiles{data: map[string]map[string]string{}} }

func (f *fakeFiles) put(pid, path, content string) *fakeFiles {
	if f.data[pid] == nil {
		f.data[pid] = map[string]string{}
	}
	f.data[pid][path] = content
	return f
}

func (f *fakeFiles) FindByPath(ctx context.Context, pid, path string) (base.File, bool, error) {
	f.finds++
	if f.findErr != nil {
		return base.File{}, false, f.findErr
	}
	if err := ctx.Err(); err != nil {
		return base.File{}, false, err
	}
	c, ok := f.data[pid][path]
	return base.File{ProjectID: pid, Path: path, Content: c, SizeBytes: len(c), UpdatedAt: f.updated}, ok, nil
}

func (f *fakeFiles) ListPaths(_ context.Context, pid string) ([]string, error) {
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []string
	for p := range f.data[pid] {
		out = append(out, p)
	}
	return out, nil
}

func uc(f *fakeFiles) *Usecase { return NewUsecase(f, base.Limits{MaxReadBytes: 1 << 20}) }

func reasonOf(t *testing.T, err error) base.Reason {
	t.Helper()
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("bukan ErrInvalid: %v", err)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	m, _ := e.Details.(map[string]any)
	r, _ := m["reason"].(base.Reason)
	return r
}

func exec(t *testing.T, u *Usecase, req readfile.Request) readfile.Response {
	t.Helper()
	res, err := u.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestInvalidProjectID(t *testing.T) {
	for _, id := range []string{"bukan-uuid", ""} {
		f := newFake()
		_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: id, Path: "a.md"})
		if r := reasonOf(t, err); r != base.ReasonInvalidProjectID {
			t.Fatalf("reason %q", r)
		}
		if f.finds+f.lists != 0 {
			t.Fatal("repo dipanggil")
		}
	}
}

func TestInvalidPath(t *testing.T) {
	cases := []struct {
		path string
		want base.Reason
	}{
		{"", base.ReasonPathEmpty},
		{"/", base.ReasonPathEmpty},
		{"//a.md", base.ReasonPathAbsolute},
		{"../x.md", base.ReasonPathTraversal},
		{"a/./b.md", base.ReasonPathTraversal},
		{"a//b.md", base.ReasonPathEmptySegment},
		{"a\x00b.md", base.ReasonPathControlChar},
		{strings.Repeat("a", 513), base.ReasonPathTooLong},
		{strings.Repeat("a", 256) + ".md", base.ReasonSegmentTooLong},
		{"\xff.md", base.ReasonInvalidEncoding},
	}
	for _, c := range cases {
		f := newFake()
		_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: c.path})
		if r := reasonOf(t, err); r != c.want {
			t.Errorf("%q: reason %q want %q", c.path, r, c.want)
			continue
		}
		if want := fmt.Sprintf("path %q tidak valid: %s", c.path, c.want.Message()); !strings.Contains(err.Error(), want) {
			t.Errorf("%q: pesan %q", c.path, err.Error())
		}
		if f.finds+f.lists != 0 {
			t.Errorf("%q: repo dipanggil", c.path)
		}
	}
}

func TestLeadingSlash(t *testing.T) {
	f := newFake().put(pidA, "docs/a.md", "hi")
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "/docs/a.md"})
	if !res.Found || res.Path != "docs/a.md" || res.Content != "hi" {
		t.Fatalf("%+v", res)
	}
}

func TestNotFound(t *testing.T) {
	res := exec(t, uc(newFake()), readfile.Request{ProjectID: pidA, Path: "a.md"})
	if res.Found || len(res.Similar) != 0 || res.Content != "" || res.TotalBytes != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestSimilar(t *testing.T) {
	f := newFake().put(pidA, "docs/auth/login.md", "x").put(pidA, "other/logn.md", "y")
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "docs/auth/logn.md"})
	if res.Found {
		t.Fatal("found")
	}
	has := func(p string) bool {
		for _, s := range res.Similar {
			if s == p {
				return true
			}
		}
		return false
	}
	if !has("docs/auth/login.md") || !has("other/logn.md") {
		t.Fatalf("%v", res.Similar)
	}
}

func TestSimilarMax5(t *testing.T) {
	f := newFake()
	for i := 0; i < 20; i++ {
		f.put(pidA, fmt.Sprintf("docs/file%d.md", i), "x")
	}
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "docs/file.md"})
	if len(res.Similar) != 5 {
		t.Fatalf("len %d", len(res.Similar))
	}
	for _, s := range res.Similar {
		if s == "docs/file.md" {
			t.Fatal("target di similar")
		}
	}
}

func TestOffsetIgnoredWhenNotFound(t *testing.T) {
	for _, off := range []int{-5, 1 << 30} {
		res := exec(t, uc(newFake()), readfile.Request{ProjectID: pidA, Path: "a.md", Offset: off})
		if res.Found {
			t.Fatal("found")
		}
	}
}

func TestSmallFile(t *testing.T) {
	f := newFake().put(pidA, "a.md", "# A\n")
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "a.md"})
	want := readfile.Response{Found: true, Path: "a.md", Content: "# A\n", SizeBytes: 4, TotalBytes: 4, NextOffset: 4}
	if fmt.Sprint(res) != fmt.Sprint(want) {
		t.Fatalf("%+v", res)
	}
}

func TestDefaultLimit(t *testing.T) {
	f := newFake().put(pidA, "a.md", strings.Repeat("a", 100000))
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "a.md"})
	if res.SizeBytes != 65536 || res.NextOffset != 65536 || !res.Truncated || res.TotalBytes != 100000 {
		t.Fatalf("%+v", res)
	}
}

func TestClamp(t *testing.T) {
	f := newFake().put(pidA, "a.md", strings.Repeat("a", 5000))
	u := NewUsecase(f, base.Limits{MaxReadBytes: 1000})
	for limit, want := range map[int]int{0: 1000, 999999: 1000, 10: 10} {
		if res := exec(t, u, readfile.Request{ProjectID: pidA, Path: "a.md", Limit: limit}); res.SizeBytes != want {
			t.Errorf("limit %d: size %d want %d", limit, res.SizeBytes, want)
		}
	}
	u0 := NewUsecase(f, base.Limits{})
	if res := exec(t, u0, readfile.Request{ProjectID: pidA, Path: "a.md", Limit: 3000}); res.SizeBytes != 3000 {
		t.Fatalf("tanpa clamp: %d", res.SizeBytes)
	}
}

func TestOffsetAtEnd(t *testing.T) {
	f := newFake().put(pidA, "a.md", "abcd")
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "a.md", Offset: 4})
	if !res.Found || res.Content != "" || res.SizeBytes != 0 || res.NextOffset != 4 || res.Truncated {
		t.Fatalf("%+v", res)
	}
}

func TestInvalidOffset(t *testing.T) {
	f := newFake().put(pidA, "a.md", "abcd")
	for _, off := range []int{5, -1} {
		_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: "a.md", Offset: off})
		if r := reasonOf(t, err); r != base.ReasonInvalidOffset {
			t.Fatalf("reason %q", r)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("offset %d di luar ukuran file (4 byte)", off)) {
			t.Fatalf("pesan %q", err.Error())
		}
	}
}

func TestEmptyFile(t *testing.T) {
	f := newFake().put(pidA, "a.md", "")
	res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "a.md"})
	if !res.Found || res.Content != "" || res.Truncated {
		t.Fatalf("%+v", res)
	}
	_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: "a.md", Offset: 1})
	if reasonOf(t, err) != base.ReasonInvalidOffset {
		t.Fatal("harus invalid_offset")
	}
}

func TestUTF8Boundaries(t *testing.T) {
	f := newFake().put(pidA, "c.md", "café résumé naïve").put(pidA, "e.md", "a😀b").put(pidA, "f.md", "café!")
	u := uc(f)

	res := exec(t, u, readfile.Request{ProjectID: pidA, Path: "c.md", Limit: 4}) // memotong di tengah é
	if !utf8.ValidString(res.Content) || res.Content != "caf" || res.NextOffset != 3 {
		t.Fatalf("%+v", res)
	}

	res = exec(t, u, readfile.Request{ProjectID: pidA, Path: "e.md", Limit: 2})
	if res.Content != "a" || res.NextOffset != 1 {
		t.Fatalf("%+v", res)
	}
	res = exec(t, u, readfile.Request{ProjectID: pidA, Path: "e.md", Offset: 1, Limit: 1})
	if res.Content != "😀" || res.NextOffset != 5 || res.SizeBytes == 0 {
		t.Fatalf("%+v", res)
	}

	res = exec(t, u, readfile.Request{ProjectID: pidA, Path: "f.md", Offset: 4})
	if res.Content != "!" || res.SizeBytes != 1 || res.TotalBytes != 6 || res.NextOffset != 6 || res.Truncated {
		t.Fatalf("%+v", res)
	}
}

func chain(t *testing.T, u *Usecase, path string, limit int, total int) string {
	t.Helper()
	var sb strings.Builder
	off := 0
	for i := 0; i < total+5; i++ {
		res := exec(t, u, readfile.Request{ProjectID: pidA, Path: path, Offset: off, Limit: limit})
		if !utf8.ValidString(res.Content) {
			t.Fatalf("potongan tidak valid UTF-8 (limit %d)", limit)
		}
		if res.TotalBytes != total {
			t.Fatalf("total %d want %d", res.TotalBytes, total)
		}
		sb.WriteString(res.Content)
		if !res.Truncated {
			return sb.String()
		}
		if res.NextOffset <= off {
			t.Fatal("tidak maju")
		}
		off = res.NextOffset
	}
	t.Fatal("loop tidak selesai")
	return ""
}

func TestChaining(t *testing.T) {
	content := strings.Repeat("Halo dunia café 😀 世界 naïve\n", 200)
	f := newFake().put(pidA, "m.md", content)
	u := uc(f)
	for _, limit := range []int{1, 2, 3, 7, 100} {
		if got := chain(t, u, "m.md", limit, len(content)); got != content {
			t.Fatalf("limit %d: isi berbeda", limit)
		}
	}
}

func TestChainingRandom(t *testing.T) {
	alphabet := []rune("ab é😀世\n ñ")
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		n := rng.Intn(60)
		rs := make([]rune, n)
		for j := range rs {
			rs[j] = alphabet[rng.Intn(len(alphabet))]
		}
		content := string(rs)
		f := newFake().put(pidA, "r.md", content)
		if got := chain(t, uc(f), "r.md", 1+rng.Intn(20), len(content)); got != content {
			t.Fatalf("iterasi %d: %q != %q", i, got, content)
		}
	}
}

func TestIsolation(t *testing.T) {
	f := newFake().put(pidA, "same.md", "isi A").put(pidB, "same.md", "isi B").
		put(pidB, "docs/login.md", "rahasia").put(pidA, "docs/x.md", "x")
	u := uc(f)
	if res := exec(t, u, readfile.Request{ProjectID: pidA, Path: "same.md"}); res.Content != "isi A" {
		t.Fatalf("%+v", res)
	}
	if res := exec(t, u, readfile.Request{ProjectID: pidB, Path: "same.md"}); res.Content != "isi B" {
		t.Fatalf("%+v", res)
	}
	res := exec(t, u, readfile.Request{ProjectID: pidA, Path: "docs/login.md"})
	if res.Found || res.Content != "" {
		t.Fatalf("bocor: %+v", res)
	}
	res = exec(t, u, readfile.Request{ProjectID: pidA, Path: "docs/logn.md"})
	for _, s := range res.Similar {
		if s == "docs/login.md" {
			t.Fatal("similar membocorkan project lain")
		}
	}
}

func TestInfraErrors(t *testing.T) {
	boom := errors.New("boom")
	f := newFake()
	f.findErr = boom
	_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: "a.md"})
	if !errors.Is(err, boom) || errors.Is(err, errs.ErrInvalid) || f.lists != 0 {
		t.Fatalf("find: %v lists=%d", err, f.lists)
	}

	f = newFake()
	f.listErr = boom
	_, err = uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: "a.md"})
	if !errors.Is(err, boom) || errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("list: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = uc(newFake()).Execute(ctx, readfile.Request{ProjectID: pidA, Path: "a.md"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx: %v", err)
	}
}

func TestWholeReturnsEntireFileBeyondMaxRead(t *testing.T) {
	f := newFake().put(pidA, "a.md", strings.Repeat("x", 100))
	u := NewUsecase(f, base.Limits{MaxReadBytes: 10})
	res := exec(t, u, readfile.Request{ProjectID: pidA, Path: "a.md", Whole: true})
	if len(res.Content) != 100 || res.TotalBytes != 100 || res.NextOffset != 100 || res.Truncated {
		t.Fatalf("%+v", res)
	}
	res = exec(t, u, readfile.Request{ProjectID: pidA, Path: "a.md"})
	if len(res.Content) != 10 || !res.Truncated {
		t.Fatalf("non-whole berubah: %+v", res)
	}
}

func TestWholeWithOffsetInvalid(t *testing.T) {
	f := newFake().put(pidA, "a.md", "hello")
	_, err := uc(f).Execute(context.Background(), readfile.Request{ProjectID: pidA, Path: "a.md", Whole: true, Offset: 5})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdatedAtFilled(t *testing.T) {
	ts := time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	f := newFake().put(pidA, "a.md", "hello")
	f.updated = ts
	for _, whole := range []bool{true, false} {
		res := exec(t, uc(f), readfile.Request{ProjectID: pidA, Path: "a.md", Whole: whole})
		if !res.UpdatedAt.Equal(ts) || res.UpdatedAt.Location() != time.UTC {
			t.Fatalf("whole=%v updated=%v", whole, res.UpdatedAt)
		}
	}
}
