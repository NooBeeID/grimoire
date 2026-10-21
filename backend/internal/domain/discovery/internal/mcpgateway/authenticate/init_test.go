package authenticate

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/verify"
)

func initDeps(ctx context.Context, rate, fail int) app.Deps {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_PROJECT_APIKEY_VERIFY, func(context.Context, any) (any, error) {
		return verify.Response{Found: true, KeyID: "k1", KeyName: "n", Prefix: "p", ProjectID: "p1"}, nil
	})
	return app.Deps{Ctx: ctx, Log: log, Bus: b, Config: config.Config{MCPRatePerMinute: rate, MCPAuthFailPerMinute: fail}}
}

func TestInit_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mw := Init(initDeps(ctx, 2, 2), silentWriter{})
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	codes := []int{}
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.Header.Set("X-API-Key", "gr_x_y")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes = append(codes, w.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Fatalf("%v", codes)
	}
}

func TestInit_NoPanicOnBounds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	for _, c := range [][2]int{{1, 1}, {120, 20}} {
		if Init(initDeps(ctx, c[0], c[1]), silentWriter{}) == nil {
			t.Fatal("nil")
		}
	}
	cancel()
}
