package base

import (
	"context"
	"testing"
)

func TestPrincipal(t *testing.T) {
	if got := (Principal{KeyName: "laptop", KeyPrefix: "gr_ab12"}).ActorLabel(); got != "laptop (gr_ab12)" {
		t.Fatal(got)
	}
	if got := (Principal{KeyPrefix: "gr_ab12"}).ActorLabel(); got != "gr_ab12" {
		t.Fatal(got)
	}
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("ctx kosong")
	}
	p := Principal{KeyID: "k", ProjectID: "p"}
	got, ok := PrincipalFrom(WithPrincipal(context.Background(), p))
	if !ok || got != p {
		t.Fatalf("%v %v", got, ok)
	}
}
