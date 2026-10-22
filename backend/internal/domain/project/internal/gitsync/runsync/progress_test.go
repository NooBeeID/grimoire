package runsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/gitprovider"
)

type sinkEv struct {
	name        string
	done, total int
	path        string
	sec         int
}

type fakeSink struct {
	mu sync.Mutex
	ev []sinkEv
}

func (f *fakeSink) add(e sinkEv)    { f.mu.Lock(); f.ev = append(f.ev, e); f.mu.Unlock() }
func (f *fakeSink) Start(total int) { f.add(sinkEv{name: "start", total: total}) }
func (f *fakeSink) Progress(d, t int, p string) {
	f.add(sinkEv{name: "progress", done: d, total: t, path: p})
}
func (f *fakeSink) Waiting(sec int, reason string) {
	f.add(sinkEv{name: "waiting", sec: sec, path: reason})
}
func (f *fakeSink) Saving() { f.add(sinkEv{name: "saving"}) }
func (f *fakeSink) names() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n []string
	for _, e := range f.ev {
		n = append(n, e.name)
	}
	return strings.Join(n, ",")
}

func start(t *testing.T, e *env) *Run {
	t.Helper()
	run, err := e.uc.Start(ctxWithID(), "p1", kreq(), base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestSinkEventsOrderAndMonotonic(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 12; i++ {
		e.addFile(fmt.Sprintf("docs/f%02d.md", i), "x")
	}
	s := &fakeSink{}
	if _, err := start(t, e).Execute(ctxWithID(), s); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.names(), "start,progress") || !strings.HasSuffix(s.names(), "progress,saving") {
		t.Fatalf("%s", s.names())
	}
	if s.ev[0].total != 12 {
		t.Fatalf("%+v", s.ev[0])
	}
	last := 0
	for _, ev := range s.ev[1 : len(s.ev)-1] {
		if ev.done != last+1 || ev.total != 12 || !strings.HasPrefix(ev.path, "docs/f") {
			t.Fatalf("progress tidak monoton: %+v last=%d", ev, last)
		}
		last = ev.done
	}
	if last != 12 {
		t.Fatalf("done akhir %d", last)
	}
}

func TestNoCandidatesStartZeroNoSaving(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/img.png", "x")
	s := &fakeSink{}
	if _, err := start(t, e).Execute(ctxWithID(), s); err != nil {
		t.Fatal(err)
	}
	if s.names() != "start" || s.ev[0].total != 0 {
		t.Fatalf("%+v", s.ev)
	}
}

func TestWaitingForwardedAndStatsAudited(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "x")
	e.git.onTree = func(onWait func(gitprovider.Wait)) {
		onWait(gitprovider.Wait{Duration: 1500 * time.Millisecond, Reason: gitprovider.WaitRateLimited})
		e.git.stats = gitprovider.Stats{Waits: 2, TotalWait: 2500 * time.Millisecond}
	}
	s := &fakeSink{}
	if _, err := start(t, e).Execute(ctxWithID(), s); err != nil {
		t.Fatal(err)
	}
	if s.ev[0].name != "waiting" || s.ev[0].sec != 2 || s.ev[0].path != "rate_limited" {
		t.Fatalf("%+v", s.ev)
	}
	ev := e.aud.events[0]
	if ev.WaitCount != 2 || ev.WaitTotal != 2500*time.Millisecond {
		t.Fatalf("%+v", ev)
	}
}

func TestRateLimitedCeilNoWriteSavedFailed(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "x")
	e.git.treeErr = &gitprovider.RateLimitedError{RetryAfter: 1500 * time.Millisecond}
	e.git.stats = gitprovider.Stats{Waits: 1, TotalWait: time.Second}
	_, err := start(t, e).Execute(ctxWithID(), NopSink{})
	_, code, _ := mapError(err)
	if code != "RATE_LIMITED" {
		t.Fatalf("%v", err)
	}
	if got := rateLimitSeconds(t, err); got != 2 {
		t.Fatalf("retry_after_seconds %d", got)
	}
	if e.c.count("upsert-files") != 0 || e.repo.saved[0].Status != "failed" || e.aud.events[0].WaitCount != 1 || e.aud.events[0].ErrorCode != "RATE_LIMITED" {
		t.Fatalf("%v %+v", e.c.list(), e.repo.saved)
	}
}

func rateLimitSeconds(t *testing.T, err error) int {
	t.Helper()
	ee := errorEvent(err)
	d, ok := ee.Details.(map[string]any)
	if !ok {
		t.Fatalf("details %#v", ee.Details)
	}
	n, _ := d["retry_after_seconds"].(int)
	return n
}

func TestAbortReleasesLockOnce(t *testing.T) {
	e := newEnv(t)
	run := start(t, e)
	run.Abort(ctxWithID())
	run.Abort(ctxWithID())
	if e.c.count("unlock") != 1 || e.c.count("tree") != 0 {
		t.Fatal(e.c.list())
	}
}

func TestExecuteOnlyOnceAndAbortAfterIsNoop(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "x")
	run := start(t, e)
	if _, err := run.Execute(ctxWithID(), NopSink{}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Execute(ctxWithID(), NopSink{}); err == nil {
		t.Fatal("Execute kedua harus gagal")
	}
	run.Abort(ctxWithID())
	if e.c.count("unlock") != 1 || e.c.count("tree") != 1 {
		t.Fatal(e.c.list())
	}
}

func TestStartFailureReleasesNothingBeforeLockAndLockAfter(t *testing.T) {
	e := newEnv(t)
	e.repo.lockOK = false
	if _, err := e.uc.Start(ctxWithID(), "p1", kreq(), base.RequestMeta{}); err == nil {
		t.Fatal("harus SYNC_IN_PROGRESS")
	}
	if e.c.count("unlock") != 0 || e.c.count("upsert") != 0 {
		t.Fatal(e.c.list())
	}
}

// ---- handler SSE ----

func sseReq(body string, ctx context.Context) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/git-sync/run", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	return req
}

func serveReq(e *env, req *http.Request, w http.ResponseWriter, opts handlerOpts) {
	r := chi.NewRouter()
	r.Post("/projects/{projectID}/git-sync/run", newHandler(e.uc, opts).ServeHTTP)
	r.ServeHTTP(w, req)
}

func eventNames(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		if n, ok := strings.CutPrefix(l, "event: "); ok {
			out = append(out, n)
		}
	}
	return out
}

func TestSSESuccess(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.addFile("docs/b.md", "b")
	rec := httptest.NewRecorder()
	serveReq(e, sseReq(okBody, ctxWithID()), rec, handlerOpts{})
	h := rec.Header()
	if rec.Code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/event-stream") || h.Get("Cache-Control") != "no-store" ||
		h.Get("X-Accel-Buffering") != "no" || h.Get("Content-Length") != "" {
		t.Fatalf("%d %v", rec.Code, h)
	}
	names := eventNames(rec.Body.String())
	if len(names) < 4 || names[0] != "start" || names[len(names)-1] != "done" || names[len(names)-2] != "saving" {
		t.Fatalf("%v", names)
	}
	b := rec.Body.String()
	if !strings.Contains(b, `data: {"total":2}`) || !strings.Contains(b, `"report"`) || strings.Contains(b, canary) {
		t.Fatalf("%s", b)
	}
}

func TestSSEPreStreamErrorIsJSON(t *testing.T) {
	e := newEnv(t)
	e.repo.lockOK = false
	rec := httptest.NewRecorder()
	serveReq(e, sseReq(okBody, ctxWithID()), rec, handlerOpts{})
	if rec.Code != 409 || strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(rec.Body.String(), "SYNC_IN_PROGRESS") {
		t.Fatalf("%d %v %s", rec.Code, rec.Header(), rec.Body)
	}
}

func TestSSEFailureAfterStartIsErrorEvent(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.blobErr = &gitprovider.RateLimitedError{RetryAfter: 61 * time.Second}
	rec := httptest.NewRecorder()
	serveReq(e, sseReq(okBody, ctxWithID()), rec, handlerOpts{})
	names := eventNames(rec.Body.String())
	b := rec.Body.String()
	if rec.Code != 200 || names[0] != "start" || names[len(names)-1] != "error" || !strings.Contains(b, `"code":"RATE_LIMITED"`) ||
		!strings.Contains(b, `"retry_after_seconds":61`) || strings.Contains(b, "event: done") {
		t.Fatalf("%d %s", rec.Code, b)
	}
}

func TestAcceptWildcardAndMixedNegotiation(t *testing.T) {
	for accept, sse := range map[string]bool{
		"*/*": false, "application/json": false, "": false, "text/event-stream": true,
		"text/html, text/event-stream;q=0.9": true, "TEXT/EVENT-STREAM": true,
	} {
		e := newEnv(t)
		e.addFile("docs/a.md", "a")
		req := sseReq(okBody, ctxWithID())
		req.Header.Set("Accept", accept)
		rec := httptest.NewRecorder()
		serveReq(e, req, rec, handlerOpts{})
		got := strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream")
		if got != sse || rec.Code != 200 {
			t.Errorf("Accept %q: sse=%v code=%d", accept, got, rec.Code)
		}
	}
}

func TestSSEHeartbeat(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.onTree = func(func(gitprovider.Wait)) { time.Sleep(120 * time.Millisecond) }
	rec := httptest.NewRecorder()
	serveReq(e, sseReq(okBody, ctxWithID()), rec, handlerOpts{heartbeat: 15 * time.Millisecond})
	if !strings.Contains(rec.Body.String(), ": ping\n\n") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("%s", rec.Body)
	}
}

type noFlushWriter struct {
	h    http.Header
	code int
	body strings.Builder
}

func (w *noFlushWriter) Header() http.Header         { return w.h }
func (w *noFlushWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *noFlushWriter) WriteHeader(c int)           { w.code = c }

func TestSSEFlushUnsupported(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	w := &noFlushWriter{h: http.Header{}}
	serveReq(e, sseReq(okBody, ctxWithID()), w, handlerOpts{})
	if w.code != 500 || strings.Contains(w.h.Get("Content-Type"), "event-stream") ||
		w.h.Get("X-Accel-Buffering") != "" || strings.Contains(w.body.String(), "event:") {
		t.Fatalf("%d %v %s", w.code, w.h, w.body.String())
	}
	if e.c.count("unlock") != 1 || e.c.count("tree") != 0 || !strings.Contains(w.body.String(), "INTERNAL") {
		t.Fatal(e.c.list(), w.body.String())
	}
}

func TestSSEPanicBecomesInternalErrorEvent(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.onTree = func(func(gitprovider.Wait)) { panic("boom secret") }
	rec := httptest.NewRecorder()
	serveReq(e, sseReq(okBody, ctxWithID()), rec, handlerOpts{})
	b := rec.Body.String()
	if !strings.Contains(b, "event: error") || !strings.Contains(b, `"code":"INTERNAL"`) || strings.Contains(b, "boom") || e.c.count("unlock") != 1 {
		t.Fatalf("%s %v", b, e.c.list())
	}
}

func TestSSEClientDisconnectSyncStillCompletes(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.git.blobGate = make(chan struct{})
	ctx, cancel := context.WithCancel(ctxWithID())
	rec := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		serveReq(e, sseReq(okBody, ctx), rec, handlerOpts{})
		close(finished)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for e.git.blobCalled.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-finished // handler kembali walau sync belum selesai
	if e.c.count("save") != 0 {
		t.Fatal("sync tidak boleh selesai sebelum gate dibuka")
	}
	close(e.git.blobGate)
	for e.c.count("unlock") == 0 && time.Now().Before(deadline.Add(2*time.Second)) {
		time.Sleep(time.Millisecond)
	}
	if e.c.count("upsert-files") != 1 || e.c.count("save") != 1 || e.c.count("unlock") != 1 || e.repo.saved[0].Status != "success" {
		t.Fatal(e.c.list())
	}
}

func TestJSONPathDetachedFromClientContext(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	ctx, cancel := context.WithCancel(ctxWithID())
	e.git.onTree = func(func(gitprovider.Wait)) { cancel() }
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/git-sync/run", strings.NewReader(okBody)).WithContext(ctx)
	rec := httptest.NewRecorder()
	serveReq(e, req, rec, handlerOpts{})
	if rec.Code != 200 || e.c.count("upsert-files") != 1 {
		t.Fatalf("%d %s %v", rec.Code, rec.Body, e.c.list())
	}
}

func TestSSESinkDropsWhenFullButNeverBlocks(t *testing.T) {
	ch := make(chan sseEvent, eventBuffer)
	s := &sseSink{ch: ch}
	finished := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			s.Progress(i+1, 500, "p")
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("sink memblok")
	}
	if len(ch) != eventBuffer {
		t.Fatalf("len %d", len(ch))
	}
}

func TestErrorEventGeneric(t *testing.T) {
	if ev := errorEvent(errors.New("pg secret")); ev.Code != "INTERNAL" || strings.Contains(ev.Message, "secret") {
		t.Fatalf("%+v", ev)
	}
	if ev := errorEvent(context.Canceled); ev.Code != "CANCELED" {
		t.Fatalf("%+v", ev)
	}
}
