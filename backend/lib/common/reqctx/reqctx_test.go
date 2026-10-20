package reqctx

import (
	"context"
	"testing"
)

func TestReqctx_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if TraceID(ctx) != "" || UserID(ctx) != "" || Role(ctx) != "" || APIKeyID(ctx) != "" {
		t.Fatal("empty ctx should return empty strings")
	}
	ctx = WithAPIKeyID(WithRole(WithUserID(WithTraceID(ctx, "t"), "u"), "r"), "k")
	if TraceID(ctx) != "t" || UserID(ctx) != "u" || Role(ctx) != "r" || APIKeyID(ctx) != "k" {
		t.Fatal("round trip failed")
	}
}

func TestReqctx_IdentitySetsUserAndRole(t *testing.T) {
	ctx := WithIdentity(context.Background(), Identity{UserID: "u1", Role: "developer", Email: "a@b"})
	if UserID(ctx) != "u1" || Role(ctx) != "developer" {
		t.Fatal("user/role not set")
	}
	id, ok := IdentityFrom(ctx)
	if !ok || id.Email != "a@b" {
		t.Fatalf("identity %+v %v", id, ok)
	}
}

func TestReqctx_IdentityFromMissing(t *testing.T) {
	if _, ok := IdentityFrom(context.Background()); ok {
		t.Fatal("expected no identity")
	}
}

func TestReqctx_Carry(t *testing.T) {
	src, cancel := context.WithCancel(context.Background())
	src = WithAPIKeyID(WithIdentity(WithTraceID(src, "t"), Identity{UserID: "u", Role: "r"}), "k")
	dst := Carry(src, context.Background())
	cancel()
	if dst.Err() != nil {
		t.Fatal("dst must not be cancelled")
	}
	if TraceID(dst) != "t" || UserID(dst) != "u" || Role(dst) != "r" || APIKeyID(dst) != "k" {
		t.Fatal("values not carried")
	}
	if _, ok := IdentityFrom(dst); !ok {
		t.Fatal("identity not carried")
	}
}
