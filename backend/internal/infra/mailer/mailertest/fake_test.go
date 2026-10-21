package mailertest

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/mailer"
)

func TestFake(t *testing.T) {
	f := &Fake{}
	if _, ok := f.Last(); ok {
		t.Fatal("belum ada pesan")
	}
	if err := f.Send(context.Background(), mailer.Message{To: "a@b.id"}); err != nil || !f.Configured() {
		t.Fatal("sukses")
	}
	if m, ok := f.Last(); !ok || m.To != "a@b.id" {
		t.Fatal("Last")
	}
	f.Err = errors.New("x")
	if f.Send(context.Background(), mailer.Message{}) == nil || len(f.Sent) != 1 {
		t.Fatal("Err tidak boleh menyimpan")
	}
	f.NotConfigured = true
	if !errors.Is(f.Send(context.Background(), mailer.Message{}), mailer.ErrNotConfigured) || f.Configured() {
		t.Fatal("NotConfigured")
	}
}
