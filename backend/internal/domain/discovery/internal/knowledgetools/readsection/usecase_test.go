package readsection

import (
	"context"
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

type fakeReader struct {
	reqs []rf.Request
	resp rf.Response
	err  error
}

func (f *fakeReader) Read(_ context.Context, req rf.Request) (rf.Response, error) {
	f.reqs = append(f.reqs, req)
	return f.resp, f.err
}

func found(content string) *fakeReader {
	return &fakeReader{resp: rf.Response{Found: true, Content: content, TotalBytes: len(content)}}
}

func exec(t *testing.T, f *fakeReader, p Params) base.ReadOutcome {
	t.Helper()
	out, err := newUsecase(f).Execute(context.Background(), "p", p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func dup(t *testing.T) string {
	t.Helper()
	return "# Intro\n## Setup\nx\n# Intro\n## Setup\ny\n"
}

func TestExecute_RequestsWhole(t *testing.T) {
	f := found("# A\n")
	exec(t, f, Params{Path: "a.md", Heading: "A", Occurrence: 1, Limit: 100})
	if len(f.reqs) != 1 || !f.reqs[0].Whole || f.reqs[0].ProjectID != "p" || f.reqs[0].Path != "a.md" {
		t.Fatalf("reqs = %+v", f.reqs)
	}
}

func TestExecute_DuplicateOccurrence(t *testing.T) {
	out := exec(t, found(dup(t)), Params{Path: "a.md", Heading: "Setup", Occurrence: 2, Limit: 1000})
	s, ok := out.(base.Section)
	if !ok || s.Heading.Line != 5 || s.Content != "## Setup\ny\n" {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_OccurrenceTooHigh(t *testing.T) {
	out := exec(t, found(dup(t)), Params{Path: "a.md", Heading: "Setup", Occurrence: 3, Limit: 1000})
	m, ok := out.(base.HeadingMiss)
	if !ok || m.Count != 2 || len(m.Similar) != 0 {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_HeadingMissHasSimilar(t *testing.T) {
	out := exec(t, found("# Setup Guide\n"), Params{Path: "a.md", Heading: "setup", Occurrence: 1, Limit: 1000})
	m, ok := out.(base.HeadingMiss)
	if !ok || m.Count != 0 || len(m.Similar) != 1 || !strings.Contains(m.Similar[0], "Setup Guide") {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_NotMarkdown(t *testing.T) {
	out := exec(t, found("# A\n"), Params{Path: "a.txt", Heading: "A", Occurrence: 1, Limit: 100})
	if _, ok := out.(base.NotMarkdown); !ok {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_NotFound(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: false, Similar: []string{"docs/a.md"}}}
	out := exec(t, f, Params{Path: "docs/b.md", Heading: "A", Occurrence: 1, Limit: 100})
	if nf, ok := out.(base.NotFound); !ok || len(nf.Similar) != 1 {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	out, err := newUsecase(&fakeReader{err: boom}).Execute(context.Background(), "p", Params{Path: "a.md", Heading: "A"})
	if out != nil || !errors.Is(err, boom) {
		t.Fatalf("out=%v err=%v", out, err)
	}
}
