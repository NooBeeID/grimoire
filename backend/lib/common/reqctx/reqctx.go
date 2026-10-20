// Package reqctx membawa nilai lintas-request (trace id, identitas) di context.
package reqctx

import "context"

type key int

const (
	keyTraceID key = iota
	keyUserID
	keyRole
	keyAPIKeyID
	keyIdentity
)

func get(ctx context.Context, k key) string {
	v, _ := ctx.Value(k).(string)
	return v
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}
func TraceID(ctx context.Context) string { return get(ctx, keyTraceID) }

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}
func UserID(ctx context.Context) string { return get(ctx, keyUserID) }

func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, keyRole, role)
}
func Role(ctx context.Context) string { return get(ctx, keyRole) }

func WithAPIKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyAPIKeyID, id)
}
func APIKeyID(ctx context.Context) string { return get(ctx, keyAPIKeyID) }

// Identity adalah identitas sesi lengkap yang ditaruh middleware auth.
type Identity struct {
	UserID, Email, Name, Role string
	MustChangePassword        bool
}

// WithIdentity menyimpan identitas dan juga mengisi UserID dan Role.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	ctx = context.WithValue(ctx, keyIdentity, id)
	ctx = WithUserID(ctx, id.UserID)
	return WithRole(ctx, id.Role)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(keyIdentity).(Identity)
	return id, ok
}

// Carry menyalin nilai propagasi dari ctx sumber ke ctx tujuan (tanpa cancel/deadline sumber).
func Carry(from, to context.Context) context.Context {
	if v := TraceID(from); v != "" {
		to = WithTraceID(to, v)
	}
	if v := UserID(from); v != "" {
		to = WithUserID(to, v)
	}
	if v := Role(from); v != "" {
		to = WithRole(to, v)
	}
	if v := APIKeyID(from); v != "" {
		to = WithAPIKeyID(to, v)
	}
	if id, ok := IdentityFrom(from); ok {
		to = context.WithValue(to, keyIdentity, id)
	}
	return to
}
