package buslistfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type fakeRepo struct {
	metas []base.Meta // urut path
	err   error

	metaCalls, pageCalls, countCalls int
	lastLike                         string
	lastLimit, lastOffset            int
}

func (f *fakeRepo) ListMeta(_ context.Context, _, like string) ([]base.Meta, error) {
	f.metaCalls++
	f.lastLike = like
	return f.metas, f.err
}

func (f *fakeRepo) ListPage(_ context.Context, _, like string, limit, offset int) ([]base.Meta, error) {
	f.pageCalls++
	f.lastLike, f.lastLimit, f.lastOffset = like, limit, offset
	if f.err != nil {
		return nil, f.err
	}
	if offset >= len(f.metas) {
		return nil, nil
	}
	return f.metas[offset:min(offset+limit, len(f.metas))], nil
}

func (f *fakeRepo) CountByPrefix(_ context.Context, _, _ string) (int, error) {
	f.countCalls++
	return len(f.metas), f.err
}

func (f *fakeRepo) calls() int { return f.metaCalls + f.pageCalls + f.countCalls }

func metas(paths ...string) []base.Meta {
	out := make([]base.Meta, 0, len(paths))
	for _, p := range paths {
		out = append(out, base.Meta{Path: p, SizeBytes: 10, UpdatedAt: time.Unix(0, 0).UTC()})
	}
	return out
}

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

func pid() string { return uuid.NewString() }

func TestInvalidProjectID(t *testing.T) {
	repo := &fakeRepo{}
	_, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: "bukan-uuid"})
	if reasonOf(t, err) != base.ReasonInvalidProjectID || repo.calls() != 0 {
		t.Fatalf("err=%v calls=%d", err, repo.calls())
	}
}

func TestPrefix(t *testing.T) {
	ok := []struct{ in, like string }{
		{"/docs", "docs/%"}, {"/", "%"}, {"", "%"}, {"docs/", "docs/%"}, {"docs", "docs/%"},
	}
	for _, c := range ok {
		repo := &fakeRepo{}
		if _, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Prefix: c.in, Recursive: true}); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if repo.lastLike != c.like {
			t.Fatalf("%q: like %q, want %q", c.in, repo.lastLike, c.like)
		}
	}
	bad := []struct {
		in     string
		reason base.Reason
	}{
		{"//docs", base.ReasonPathAbsolute}, {"../x", base.ReasonPathTraversal}, {"a//b", base.ReasonPathEmptySegment},
	}
	for _, c := range bad {
		repo := &fakeRepo{}
		_, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Prefix: c.in})
		if reasonOf(t, err) != c.reason || repo.calls() != 0 {
			t.Fatalf("%q: err=%v", c.in, err)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("%q", c.in)) || !strings.Contains(err.Error(), c.reason.Message()) {
			t.Fatalf("%q: pesan %q", c.in, err.Error())
		}
	}
}

func TestLimitOffset(t *testing.T) {
	for _, c := range [][2]int{{0, 100}, {9999, 500}, {7, 7}} {
		repo := &fakeRepo{}
		_, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Recursive: true, Limit: c[0]})
		if err != nil || repo.lastLimit != c[1] {
			t.Fatalf("limit %d -> %d (%v)", c[0], repo.lastLimit, err)
		}
	}
	repo := &fakeRepo{}
	_, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Offset: -1})
	if reasonOf(t, err) != base.ReasonInvalidOffset || repo.calls() != 0 {
		t.Fatalf("err=%v", err)
	}
	if _, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Offset: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestRecursivePaging(t *testing.T) {
	all := metas("a.md", "b.md", "c.md", "d.md", "e.md")
	cases := []struct {
		offset, items int
		trunc         bool
	}{{0, 2, true}, {4, 1, false}, {5, 0, false}}
	for _, c := range cases {
		repo := &fakeRepo{metas: all}
		res, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Recursive: true, Limit: 2, Offset: c.offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != c.items || res.Truncated != c.trunc || res.Total != 5 || res.Folders != nil || res.Items == nil {
			t.Fatalf("offset %d: %+v", c.offset, res)
		}
		if repo.lastOffset != c.offset || repo.lastLimit != 2 || repo.metaCalls != 0 {
			t.Fatalf("repo args: %+v", repo)
		}
	}
	res, err := New(&fakeRepo{}).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Recursive: true})
	if err != nil || res.Items == nil || len(res.Items) != 0 || res.Truncated || res.Total != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestNonRecursive(t *testing.T) {
	all := metas("docs/a.md", "docs/auth/flow/x.md", "docs/auth/login.md", "docs/b.md")
	run := func(limit, offset int) (listfiles.Response, *fakeRepo) {
		repo := &fakeRepo{metas: all}
		res, err := New(repo).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Prefix: "docs", Limit: limit, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		return res, repo
	}
	res, repo := run(0, 0)
	if len(res.Items) != 2 || res.Items[0].Path != "docs/a.md" || res.Items[1].Path != "docs/b.md" ||
		len(res.Folders) != 1 || res.Folders[0] != "docs/auth" || res.Total != 2 || res.Truncated {
		t.Fatalf("%+v", res)
	}
	if repo.pageCalls != 0 || repo.countCalls != 0 {
		t.Fatal("ListPage/Count dipanggil")
	}
	res, _ = run(1, 0)
	if len(res.Items) != 1 || res.Items[0].Path != "docs/a.md" || !res.Truncated || len(res.Folders) != 1 {
		t.Fatalf("%+v", res)
	}
	res, _ = run(1, 1)
	if len(res.Items) != 1 || res.Items[0].Path != "docs/b.md" || res.Truncated {
		t.Fatalf("%+v", res)
	}
	res, _ = run(1, 10)
	if res.Items == nil || len(res.Items) != 0 || res.Truncated || res.Folders == nil {
		t.Fatalf("%+v", res)
	}
	res, _ = (&Usecase{repo: &fakeRepo{}}).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Prefix: "nope"})
	if res.Items == nil || res.Folders == nil || res.Total != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestRepoError(t *testing.T) {
	sentinel := errors.New("pq: relation boom")
	for _, rec := range []bool{true, false} {
		_, err := New(&fakeRepo{err: sentinel}).Execute(context.Background(), listfiles.Request{ProjectID: pid(), Recursive: rec})
		if !errors.Is(err, sentinel) || errors.Is(err, errs.ErrInvalid) {
			t.Fatalf("rec=%v err=%v", rec, err)
		}
	}
}
