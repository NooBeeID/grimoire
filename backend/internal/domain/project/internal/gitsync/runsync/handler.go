package runsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

const eventBuffer = 64

type handlerOpts struct {
	timeout   time.Duration // batas total Execute (ctx lepas dari klien)
	heartbeat time.Duration
}

type handler struct {
	uc *Usecase
	handlerOpts
}

func newHandler(uc *Usecase, o handlerOpts) *handler {
	if o.timeout <= 0 {
		o.timeout = 15 * time.Minute
	}
	if o.heartbeat <= 0 {
		o.heartbeat = 15 * time.Second
	}
	return &handler{uc: uc, handlerOpts: o}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in RunRequest
	if err := respond.DecodeJSON(r, &in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
	run, err := h.uc.Start(r.Context(), chi.URLParam(r, "projectID"), in, meta)
	if err != nil {
		setRetryAfter(w, err)
		respond.WriteError(w, r, err)
		return
	}
	if wantsSSE(r) {
		h.serveSSE(w, r, run)
		return
	}
	h.serveJSON(w, r, run)
}

// wantsSSE: true hanya bila Accept memuat tepat text/event-stream ("*/*" tidak memicu SSE).
func wantsSSE(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept") {
		for _, part := range strings.Split(v, ",") {
			mt, _, _ := strings.Cut(part, ";")
			if strings.EqualFold(strings.TrimSpace(mt), "text/event-stream") {
				return true
			}
		}
	}
	return false
}

// execCtx melepas Execute dari pembatalan klien tetapi tetap dibatasi timeout total.
func (h *handler) execCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), h.timeout)
}

func (h *handler) serveJSON(w http.ResponseWriter, r *http.Request, run *Run) {
	ectx, cancel := h.execCtx(r)
	defer cancel()
	res, err := run.Execute(ectx, NopSink{})
	if err != nil {
		setRetryAfter(w, err)
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, http.StatusOK, res)
}

type sseEvent struct {
	name string
	data any
}

type outcome struct {
	res RunResponse
	err error
}

// sseSink meneruskan event ke kanal ber-buffer; event dibuang bila penuh agar sync tidak terblokir.
type sseSink struct{ ch chan sseEvent }

func (s *sseSink) send(name string, data any) {
	select {
	case s.ch <- sseEvent{name, data}:
	default:
	}
}
func (s *sseSink) Start(total int) { s.send(EventStart, StartEvent{Total: total}) }
func (s *sseSink) Progress(done, total int, path string) {
	s.send(EventProgress, ProgressEvent{Done: done, Total: total, Path: path})
}
func (s *sseSink) Waiting(sec int, reason string) {
	s.send(EventWaiting, WaitingEvent{RetryAfterSeconds: sec, Reason: reason})
}
func (s *sseSink) Saving() { s.send(EventSaving, SavingEvent{}) }

func (h *handler) serveSSE(w http.ResponseWriter, r *http.Request, run *Run) {
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream; charset=utf-8")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no")
	hd.Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)
	ferr := rc.Flush()
	if errors.Is(ferr, http.ErrNotSupported) {
		hd.Del("Content-Type")
		hd.Del("X-Accel-Buffering")
		hd.Del("Connection")
		run.Abort(r.Context())
		respond.WriteError(w, r, &errs.Error{Code: errs.CodeInternal, Message: genericInternal, Err: ferr})
		return
	}

	events := make(chan sseEvent, eventBuffer)
	done := make(chan outcome, 1)
	sink := &sseSink{ch: events}
	go func() {
		var out outcome
		defer func() {
			if p := recover(); p != nil {
				out = outcome{err: &errs.Error{Code: errs.CodeInternal, Message: genericInternal, Err: fmt.Errorf("runsync: panic: %v", p)}}
			}
			done <- out
		}()
		ectx, cancel := h.execCtx(r)
		defer cancel()
		out.res, out.err = run.Execute(ectx, sink)
	}()
	if ferr != nil {
		return // klien sudah pergi; sync tetap selesai di goroutine
	}

	tick := time.NewTicker(h.heartbeat)
	defer tick.Stop()
	write := func(name string, data any) bool {
		b, err := json.Marshal(data)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	for {
		select {
		case ev := <-events:
			if !write(ev.name, ev.data) {
				return
			}
		case out := <-done:
			for drained := false; !drained; {
				select {
				case ev := <-events:
					if !write(ev.name, ev.data) {
						return
					}
				default:
					drained = true
				}
			}
			if out.err != nil {
				write(EventError, errorEvent(out.err))
			} else {
				write(EventDone, out.res)
			}
			return
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// errorEvent membentuk payload error tanpa membocorkan detail internal.
func errorEvent(err error) ErrorEvent {
	var ee *errs.Error
	switch {
	case errors.As(err, &ee):
		if ee.Code == errs.CodeInternal {
			return ErrorEvent{Code: string(errs.CodeInternal), Message: genericInternal}
		}
		return ErrorEvent{Code: string(ee.Code), Message: ee.Message, Details: ee.Details}
	case errors.Is(err, context.Canceled):
		return ErrorEvent{Code: "CANCELED", Message: "sync dibatalkan"}
	}
	return ErrorEvent{Code: string(errs.CodeInternal), Message: genericInternal}
}

// setRetryAfter menyalin details.retry_after_seconds ke header Retry-After.
func setRetryAfter(w http.ResponseWriter, err error) {
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeRateLimited {
		return
	}
	d, ok := e.Details.(map[string]any)
	if !ok {
		return
	}
	if n, ok := d["retry_after_seconds"].(int); ok && n > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(n))
	}
}
