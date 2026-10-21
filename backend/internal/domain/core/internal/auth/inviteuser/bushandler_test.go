package inviteuser

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/invitation"
	contract "grimoire/lib/contracts/core/auth/inviteuser"
)

func busEnv(e env) *bus.Registry {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(&discard{}, nil)))
	b.Register("core.auth.invite-user", NewBusHandler(e.uc).Handle)
	return b
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func busReq(email string, deliver bool) contract.Request {
	return contract.Request{Email: email, Name: "Budi", InvitedBy: "owner-1", InvitedByLabel: "Owner Satu", ProjectName: "Alpha", Deliver: deliver}
}

func TestBus_Handle_ReturnsValueResponse(t *testing.T) {
	e := setup()
	res, err := bus.CallTyped[contract.Response](context.Background(), busEnv(e), "core.auth.invite-user", busReq("a@x.id", false))
	if err != nil {
		t.Fatal(err)
	}
	if res.UserID != "new-id" || !res.Created || res.EmailStatus != invitation.EmailStatusPending || res.EmailSent || res.Credentials != nil ||
		!res.ExpiresAt.Equal(t0.Add(testPolicy.TTL)) {
		t.Fatalf("res: %+v", res)
	}
	if len(e.s.calls) != 0 {
		t.Fatal("Deliver=false tidak boleh mengirim")
	}
}

func TestBus_Handle_BadRequestType(t *testing.T) {
	b := busEnv(setup())
	r := busReq("a@x.id", false)
	for _, req := range []any{"salah", &r} {
		if _, err := b.Call(context.Background(), "core.auth.invite-user", req); !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestBus_Handle_PropagatesErrsError(t *testing.T) {
	cases := map[string]func(e env) string{
		"ALREADY_REGISTERED": func(e env) string {
			e.r.user = &base.User{ID: "u", Role: base.RoleSuperAdmin, IsActive: true}
			return "a@x.id"
		},
		"RATE_LIMITED":     func(e env) string { e.r.count, e.r.oldest = 20, t0.Add(-time.Minute); return "a@x.id" },
		"INVALID_ARGUMENT": func(env) string { return "bukan-email" },
	}
	for code, prep := range cases {
		t.Run(code, func(t *testing.T) {
			e := setup()
			email := prep(e)
			_, err := busEnv(e).Call(context.Background(), "core.auth.invite-user", busReq(email, false))
			var ee *errs.Error
			if !errors.As(err, &ee) || string(ee.Code) != code || ee.Details == nil {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestBus_Handle_DeliverTrueFallbackCredentials(t *testing.T) {
	e := setup()
	e.s.status = base.DeliveryFailed
	res, err := bus.CallTyped[contract.Response](context.Background(), busEnv(e), "core.auth.invite-user", busReq("a@x.id", true))
	if err != nil {
		t.Fatal(err)
	}
	if res.EmailSent || res.EmailStatus != "failed" || res.Credentials == nil || len(res.Credentials.TemporaryPassword) != 16 {
		t.Fatalf("res: %+v", res)
	}
}

func TestBus_Handle_ViaProjectAndProjectNameInAudit(t *testing.T) {
	e := setup()
	if _, err := busEnv(e).Call(context.Background(), "core.auth.invite-user", busReq("a@x.id", false)); err != nil {
		t.Fatal(err)
	}
	a := e.a.events[0]
	if a.Args["via"] != "project" || a.Args["project_name"] != "Alpha" || a.Meta != (base.RequestMeta{}) {
		t.Fatalf("event: %+v", a)
	}
}
