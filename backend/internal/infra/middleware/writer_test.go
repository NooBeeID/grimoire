package middleware

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// bareWriter tidak mengimplementasikan Flusher.
type bareWriter struct{ h http.Header }

func (b *bareWriter) Header() http.Header         { return b.h }
func (b *bareWriter) Write(p []byte) (int, error) { return len(p), nil }
func (b *bareWriter) WriteHeader(int)             {}

func TestStatusWriter_FlushErrorNotSupported(t *testing.T) {
	sw := newStatusWriter(&bareWriter{h: http.Header{}})
	err := http.NewResponseController(sw).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("got %v", err)
	}
	if sw.wroteHeader {
		t.Fatal("header tidak boleh ditulis")
	}
}

func TestStatusWriter_FlushErrorSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)
	if err := http.NewResponseController(sw).Flush(); err != nil {
		t.Fatal(err)
	}
	if !rec.Flushed || !sw.wroteHeader {
		t.Fatalf("flushed=%v wrote=%v", rec.Flushed, sw.wroteHeader)
	}
}

func TestStatusWriter_StreamThroughChain(t *testing.T) {
	release := make(chan struct{})
	h := AccessLog(slog.New(slog.NewTextHandler(io.Discard, nil)))(
		Timeout(time.Second)(ExtendTimeout(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: a\n\n")
			if err := rc.Flush(); err != nil {
				t.Errorf("flush: %v", err)
			}
			<-release
			_, _ = io.WriteString(w, "event: b\n\n")
		}))))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil || line != "event: a\n" {
		t.Fatalf("event pertama %q %v", line, err)
	}
	close(release)
	_, _ = io.ReadAll(br)
}
