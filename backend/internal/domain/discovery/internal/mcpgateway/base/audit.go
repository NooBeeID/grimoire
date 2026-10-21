package base

import (
	"time"

	"grimoire/lib/contracts/core/audit/record"
)

func ms(d time.Duration) int64 { return d.Milliseconds() }

func keyRequest(p Principal, action string) record.Request {
	return record.Request{
		ActorType: record.ActorAPIKey, ActorID: p.KeyID, ActorLabel: p.ActorLabel(),
		ProjectID: p.ProjectID, ProjectLabel: p.ProjectName,
		Action: action, IP: p.IP, UserAgent: p.UserAgent,
	}
}

// ToolCallAudit membangun audit satu panggilan tool. Text tidak pernah masuk.
func ToolCallAudit(p Principal, tool string, args map[string]any, o Outcome, dur time.Duration) record.Request {
	r := keyRequest(p, "mcp."+tool)
	r.TargetType, r.Target = o.Result.TargetType, o.Result.Target
	r.Args = args
	res := make(map[string]any, len(o.Result.Summary)+5)
	for k, v := range o.Result.Summary {
		res[k] = v
	}
	res["bytes"] = len(o.Result.Text)
	res["original_bytes"] = o.OriginalBytes
	res["truncated"] = o.Truncated
	res["duration_ms"] = ms(dur)
	// error_code dari tool dipertahankan; kode dari gateway menimpanya.
	if o.ErrorCode != "" {
		res["error_code"] = o.ErrorCode
	}
	r.Result = res
	r.Status = record.StatusOK
	if o.Result.IsError {
		r.Status = record.StatusError
	}
	return r
}

// UnknownToolAudit membangun audit tool tak dikenal.
func UnknownToolAudit(p Principal, name string, dur time.Duration) record.Request {
	name = TruncateName(name, 128)
	r := keyRequest(p, record.ActionMCPUnknownTool)
	r.TargetType, r.Target = "tool", name
	r.Args = map[string]any{"name": name}
	r.Result = map[string]any{"error_code": "unknown_tool", "duration_ms": ms(dur)}
	r.Status = record.StatusError
	return r
}

// InitializeAudit membangun audit initialize.
func InitializeAudit(p Principal, requested, negotiated, clientName, clientVersion string, ok bool, dur time.Duration) record.Request {
	r := keyRequest(p, record.ActionMCPInitialize)
	r.Args = map[string]any{"protocol_version": requested, "client_name": clientName, "client_version": clientVersion}
	if ok {
		r.Result = map[string]any{"protocol_version": negotiated, "duration_ms": ms(dur)}
		r.Status = record.StatusOK
	} else {
		r.Result = map[string]any{"error_code": "unsupported_version", "duration_ms": ms(dur)}
		r.Status = record.StatusError
	}
	return r
}

// ToolsListAudit membangun audit tools/list.
func ToolsListAudit(p Principal, count int, dur time.Duration) record.Request {
	r := keyRequest(p, record.ActionMCPToolsList)
	r.Args = map[string]any{}
	r.Result = map[string]any{"count": count, "duration_ms": ms(dur)}
	r.Status = record.StatusOK
	return r
}

// AuthRejectedAudit membangun audit autentikasi ditolak.
func AuthRejectedAudit(ip, ua, prefix, reason string) record.Request {
	r := record.Request{
		ActorType: record.ActorAnonymous, ActorLabel: prefix,
		Action: record.ActionMCPAuthRejected,
		Args:   map[string]any{"reason": reason, "key_prefix": prefix},
		Result: map[string]any{}, Status: record.StatusDenied, IP: ip, UserAgent: ua,
	}
	if prefix != "" {
		r.TargetType, r.Target = "api_key", prefix
	}
	return r
}

// RateLimitedKeyAudit membangun audit rate limit per key.
func RateLimitedKeyAudit(p Principal, retryAfter int) record.Request {
	r := keyRequest(p, record.ActionMCPRateLimited)
	r.TargetType, r.Target = "api_key", p.KeyID
	r.Args = map[string]any{"scope": "key", "retry_after_seconds": retryAfter}
	r.Result = map[string]any{}
	r.Status = record.StatusDenied
	return r
}

// RateLimitedIPAudit membangun audit rate limit gagal-auth per IP.
func RateLimitedIPAudit(ip, ua string, retryAfter int) record.Request {
	return record.Request{
		ActorType: record.ActorAnonymous, Action: record.ActionMCPRateLimited,
		TargetType: "ip", Target: ip,
		Args:   map[string]any{"scope": "auth_ip", "retry_after_seconds": retryAfter},
		Result: map[string]any{}, Status: record.StatusDenied, IP: ip, UserAgent: ua,
	}
}
