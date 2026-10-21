package base

import "context"

// Principal adalah identitas hasil autentikasi API key untuk satu request.
type Principal struct {
	KeyID, KeyName, KeyPrefix string
	ProjectID, ProjectName    string
	IP, UserAgent             string
}

// ActorLabel "<KeyName> (<KeyPrefix>)"; KeyName kosong -> KeyPrefix.
func (p Principal) ActorLabel() string {
	if p.KeyName == "" {
		return p.KeyPrefix
	}
	return p.KeyName + " (" + p.KeyPrefix + ")"
}

type principalKey struct{}

// WithPrincipal menyimpan Principal di context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom membaca Principal dari context.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
