package httpx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestServe_GracefulWaitsInflight(t *testing.T) {
	log, _ := useLogger(t)
	started := make(chan struct{})
	srv := NewServer(testConfig(t, nil), log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	}))
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, srv, ln, 5*time.Second, log) }()

	type res struct {
		code int
		body string
		err  error
	}
	out := make(chan res, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			out <- res{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		out <- res{code: resp.StatusCode, body: string(b)}
	}()
	<-started
	cancel()
	r := <-out
	if r.err != nil || r.code != 200 || r.body != "done" {
		t.Fatalf("%+v", r)
	}
	if err := <-serveErr; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestServe_ShutdownTimeoutForcesClose(t *testing.T) {
	log, _ := useLogger(t)
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	srv := NewServer(testConfig(t, nil), log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, srv, ln, 50*time.Millisecond, log) }()
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-started
	cancel()
	select {
	case err := <-serveErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve menggantung")
	}
}

func TestServe_ListenErrorReturned(t *testing.T) {
	log, _ := useLogger(t)
	ln := listen(t)
	defer ln.Close()
	cfg := testConfig(t, map[string]string{"HTTP_ADDR": ln.Addr().String()})
	srv := NewServer(cfg, log, http.NewServeMux())
	if err := Serve(context.Background(), srv, nil, time.Second, log); err == nil {
		t.Fatal("harus error karena port dipakai")
	}
}

func TestRunShutdown_OrderAndContinueOnError(t *testing.T) {
	log, _ := useLogger(t)
	var order []string
	boom := errors.New("jobs gagal")
	step := func(name string, err error, to time.Duration) ShutdownStep {
		return ShutdownStep{Name: name, Timeout: to, Fn: func(ctx context.Context) error {
			order = append(order, name)
			dl, ok := ctx.Deadline()
			if !ok || time.Until(dl) > to {
				t.Errorf("%s: deadline tidak sesuai", name)
			}
			return err
		}}
	}
	err := RunShutdown(log, step("jobs", boom, time.Second), step("bus", nil, 2*time.Second), step("db", nil, time.Second))
	if len(order) != 3 || order[0] != "jobs" || order[1] != "bus" || order[2] != "db" {
		t.Fatalf("%v", order)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if err := RunShutdown(log, step("x", nil, time.Second)); err != nil {
		t.Fatal(err)
	}
}
