package runsync

import (
	"bufio"
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/middleware"
)

// Rantai nyata: AccessLog (statusWriter) -> Timeout -> ExtendTimeout -> handler. Membuktikan Flush
// menembus pembungkus writer sehingga klien menerima event sebelum sync selesai.
func TestRouterRealChainFlushesBeforeFinish(t *testing.T) {
	e := newEnv(t)
	e.addFile("docs/a.md", "a")
	e.addFile("docs/b.md", "b")
	e.git.blobGate = make(chan struct{})
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	r := chi.NewRouter()
	r.Use(middleware.AccessLog(logger), middleware.Timeout(5*time.Second))
	r.With(middleware.ExtendTimeout(time.Minute)).Post("/projects/{projectID}/git-sync/run",
		newHandler(e.uc, handlerOpts{heartbeat: time.Hour}).ServeHTTP)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.ServeHTTP(w, req)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/projects/p1/git-sync/run", strings.NewReader(okBody))
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("%d %v", resp.StatusCode, resp.Header)
	}
	sc := bufio.NewScanner(resp.Body)
	gotStart := make(chan struct{})
	go func() {
		for sc.Scan() {
			if sc.Text() == "event: start" {
				close(gotStart)
				return
			}
		}
	}()
	select {
	case <-gotStart: // diterima sementara blob masih ditahan
	case <-time.After(3 * time.Second):
		t.Fatal("event start tidak sampai ke klien sebelum sync selesai")
	}
	close(e.git.blobGate)
	rest := &strings.Builder{}
	for sc.Scan() {
		rest.WriteString(sc.Text() + "\n")
	}
	if !strings.Contains(rest.String(), "event: done") {
		t.Fatalf("%s", rest.String())
	}
	if !strings.Contains(logBuf.String(), `"status":200`) {
		t.Fatalf("accesslog: %s", logBuf.String())
	}
}
