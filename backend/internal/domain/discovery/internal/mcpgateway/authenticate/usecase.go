package authenticate

import (
	"context"
	"log/slog"
	"math"
	"time"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/touch"
	"grimoire/lib/contracts/project/apikey/verify"
	"grimoire/lib/contracts/project/management/getproject"
)

// Limiter dipenuhi *middleware.Limiter.
type Limiter interface {
	Allow(key string) (bool, time.Duration)
	Peek(key string) (bool, time.Duration)
}

// Throttle dipenuhi *base.AuditThrottle.
type Throttle interface{ Allow(subject string) bool }

type Kind int

const (
	Admitted     Kind = iota + 1
	Unauthorized      // 401
	RateLimited       // 429
	Unavailable       // 503
)

// Decision sengaja tidak memuat Reason (respons ke luar harus seragam).
type Decision struct {
	Kind       Kind
	Principal  base.Principal // hanya bila Admitted
	RetryAfter int            // detik, >= 1, hanya bila RateLimited
}

type Usecase struct {
	bus      *bus.Registry
	ipLim    Limiter
	keyLim   Limiter
	throttle Throttle
	audit    auditwriter.Writer
	log      *slog.Logger
}

func NewUsecase(b *bus.Registry, ipLim, keyLim Limiter, th Throttle, audit auditwriter.Writer, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{bus: b, ipLim: ipLim, keyLim: keyLim, throttle: th, audit: audit, log: log}
}

func (u *Usecase) Execute(ctx context.Context, in Input) Decision {
	if ok, wait := u.ipLim.Peek(in.IP); !ok {
		return u.limited(ctx, wait, in, nil)
	}

	var res verify.Response
	reason, prefix := "", ""
	if in.RawKey == "" {
		reason = "missing"
	} else {
		var err error
		res, err = bus.CallTyped[verify.Response](ctx, u.bus, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: in.RawKey})
		if err != nil {
			u.log.ErrorContext(ctx, "mcp verify failed", "err", err)
			return Decision{Kind: Unavailable}
		}
		if !res.Found {
			reason, prefix = res.Reason, res.Prefix
		}
	}

	if reason != "" {
		u.ipLim.Allow(in.IP)
		u.audit.Write(ctx, base.AuthRejectedAudit(in.IP, in.UserAgent, prefix, reason))
		return Decision{Kind: Unauthorized}
	}

	if ok, wait := u.keyLim.Allow(res.KeyID); !ok {
		return u.limited(ctx, wait, in, &res)
	}

	p := u.principal(ctx, res, in)
	if err := u.bus.Publish(context.WithoutCancel(ctx), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: res.KeyID}); err != nil {
		u.log.WarnContext(ctx, "mcp touch publish failed", "key_id", res.KeyID, "err", err)
	}
	return Decision{Kind: Admitted, Principal: p}
}

func (u *Usecase) principal(ctx context.Context, res verify.Response, in Input) base.Principal {
	return base.Principal{
		KeyID: res.KeyID, KeyName: res.KeyName, KeyPrefix: res.Prefix,
		ProjectID: res.ProjectID, ProjectName: u.projectName(ctx, res.ProjectID),
		IP: in.IP, UserAgent: in.UserAgent,
	}
}

// projectName fail-soft: kegagalan hanya mengosongkan label.
func (u *Usecase) projectName(ctx context.Context, projectID string) string {
	gp, err := bus.CallTyped[getproject.Response](ctx, u.bus, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: projectID})
	if err != nil || !gp.Found {
		u.log.WarnContext(ctx, "mcp get-project failed", "project_id", projectID, "err", err)
		return ""
	}
	return gp.Name
}

func (u *Usecase) limited(ctx context.Context, wait time.Duration, in Input, res *verify.Response) Decision {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	if res == nil {
		if u.throttle.Allow("ip:" + in.IP) {
			u.audit.Write(ctx, base.RateLimitedIPAudit(in.IP, in.UserAgent, secs))
		} else {
			u.log.WarnContext(ctx, "mcp rate limited (audit throttled)", "scope", "auth_ip")
		}
	} else {
		if u.throttle.Allow("key:" + res.KeyID) {
			u.audit.Write(ctx, base.RateLimitedKeyAudit(u.principal(ctx, *res, in), secs))
		} else {
			u.log.WarnContext(ctx, "mcp rate limited (audit throttled)", "scope", "key")
		}
	}
	return Decision{Kind: RateLimited, RetryAfter: secs}
}
