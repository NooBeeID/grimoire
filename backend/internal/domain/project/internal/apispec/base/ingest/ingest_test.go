package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
)

const pid = "0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90"

const specOK = "openapi: 3.0.3\ninfo: { title: Billing, version: 1.0.0 }\npaths:\n  /a:\n    get:\n      responses: { \"200\": { description: OK } }\n"

type repo struct {
	specs map[string]base.Spec
	txs   int
}

type tx struct{ r *repo }

func (r *repo) InTx(_ context.Context, fn func(Tx) error) error { r.txs++; return fn(&tx{r}) }
func (t *tx) LockProjectSpecs(context.Context, string) error    { return nil }
func (t *tx) LockSpec(_ context.Context, _, n string) (base.Spec, bool, error) {
	s, ok := t.r.specs[n]
	return s, ok, nil
}
func (t *tx) CountSpecs(context.Context, string) (int, error) { return len(t.r.specs), nil }
func (t *tx) InsertSpec(_ context.Context, s *base.Spec) error {
	s.ID = "id-" + s.Name
	t.r.specs[s.Name] = *s
	return nil
}
func (t *tx) UpdateSpecContent(_ context.Context, s base.Spec) error {
	t.r.specs[s.Name] = s
	return nil
}
func (t *tx) ReplaceEndpoints(context.Context, string, []base.Endpoint) error { return nil }

type rec struct{ events []audit.Event }

func (r *rec) Record(_ context.Context, ev audit.Event) { r.events = append(r.events, ev) }

func newDeps() (Deps, *repo, *rec) {
	r, a := &repo{specs: map[string]base.Spec{}}, &rec{}
	return Deps{Repo: r, Audit: a, Clock: func() time.Time { return time.Unix(0, 0) }, MaxSpecs: 2, MaxFileBytes: 100}, r, a
}

func TestProcessFile_GitSourceAuditsWithActor(t *testing.T) {
	d, _, a := newDeps()
	actor := base.Actor{ID: "u1", Label: "Rina"}
	req := Request{ProjectID: pid, Actor: actor, Source: "git_sync"}
	fr, err := ProcessFile(context.Background(), d, req, upload.File{FileName: "billing.yaml", RawPath: "api/openapi/billing.yaml", Content: []byte(specOK)}, "", map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if fr.Status != base.StatusCreated || fr.Name != "billing" || fr.EndpointCount != 1 {
		t.Fatalf("%+v", fr)
	}
	if len(a.events) != 1 {
		t.Fatalf("events=%d", len(a.events))
	}
	ev := a.events[0]
	if ev.Actor == nil || *ev.Actor != actor || ev.Args["source"] != "git_sync" || ev.Target != "billing" {
		t.Fatalf("%+v", ev)
	}
	if strings.Contains(ev.Args["source_path"].(string), "..") {
		t.Fatal()
	}
}

func TestProcessFile_UploadSourceHasNoSourceArg(t *testing.T) {
	d, _, a := newDeps()
	_, err := ProcessFile(context.Background(), d, Request{ProjectID: pid, Actor: base.Actor{ID: "u1"}},
		upload.File{FileName: "a.yaml", RawPath: "a.yaml", Content: []byte(specOK)}, "", map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := a.events[0].Args["source"]; has {
		t.Fatal("upload tidak boleh punya source")
	}
}

func TestProcessFile_RejectsAndSkips(t *testing.T) {
	d, r, a := newDeps()
	req := Request{ProjectID: pid, Actor: base.Actor{ID: "u1"}}
	res := map[string]struct{}{}
	run := func(f upload.File) base.FileResult {
		fr, err := ProcessFile(context.Background(), d, req, f, "", res)
		if err != nil {
			t.Fatal(err)
		}
		return fr
	}
	if fr := run(upload.File{FileName: "x.md", RawPath: "x.md", Content: []byte("x")}); fr.Status != base.StatusSkipped || fr.ReasonCode != base.ReasonNotSpecFile {
		t.Fatalf("%+v", fr)
	}
	if fr := run(upload.File{FileName: "a.yaml", RawPath: "a.yaml", TooLarge: true}); fr.Status != base.StatusRejected || fr.ReasonCode != base.ReasonFileTooLarge {
		t.Fatalf("%+v", fr)
	}
	if fr := run(upload.File{FileName: "bad.yaml", RawPath: "bad.yaml", Content: []byte("foo: bar")}); fr.Status != base.StatusRejected || fr.ReasonCode != base.ReasonInvalidSpec || len(fr.Issues) == 0 {
		t.Fatalf("%+v", fr)
	}
	ok := upload.File{FileName: "a.yaml", RawPath: "a.yaml", Content: []byte(specOK)}
	if fr := run(ok); fr.Status != base.StatusCreated {
		t.Fatalf("%+v", fr)
	}
	if fr := run(ok); fr.Status != base.StatusRejected || fr.ReasonCode != base.ReasonDuplicateNameInUpload {
		t.Fatalf("%+v", fr)
	}
	if r.txs != 1 || len(a.events) != 1 {
		t.Fatalf("txs=%d events=%d", r.txs, len(a.events))
	}
	// unchanged tidak diaudit
	res2 := map[string]struct{}{}
	fr, _ := ProcessFile(context.Background(), d, req, ok, "", res2)
	if fr.Status != base.StatusUnchanged || len(a.events) != 1 {
		t.Fatalf("%+v events=%d", fr, len(a.events))
	}
}
