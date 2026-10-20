package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestWithDomains_RegistersAuthEndpointsAndTearsDown(t *testing.T) {
	url, _, _ := testDBURL(t)
	cfg := testCfg(t, url, nil)
	var deps app.Deps
	err := WithDomains(context.Background(), cfg, quietLog(), func(d app.Deps) error {
		deps = d
		_, err := d.Bus.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, "salah")
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Errorf("err = %v (handler belum terdaftar?)", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.Ping(context.Background()); err == nil {
		t.Error("db harus tertutup setelah WithDomains")
	}
	if _, err := deps.Bus.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, "x"); err == nil {
		// Call sinkron tetap jalan setelah Shutdown; yang dijamin hanya Publish ditolak.
		t.Log("Call setelah Shutdown masih diizinkan")
	}
}

func TestWithDomains_FnErrorPreserved(t *testing.T) {
	url, _, _ := testDBURL(t)
	cfg := testCfg(t, url, nil)
	want := errs.New(errs.CodeConflict, "email sudah terdaftar")
	err := WithDomains(context.Background(), cfg, quietLog(), func(app.Deps) error { return want })
	var ee *errs.Error
	if !errors.As(err, &ee) || ee != want {
		t.Fatalf("err = %v", err)
	}
}

func TestWithDomains_DBUnreachable(t *testing.T) {
	cfg := testCfg(t, "postgres://u:p@127.0.0.1:1/x?sslmode=disable", nil)
	called := false
	err := WithDomains(context.Background(), cfg, quietLog(), func(app.Deps) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "open db:") || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
