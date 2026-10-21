package delivery

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/mailer"
	"grimoire/internal/infra/mailer/mailertest"
)

func msg() mailer.Message {
	return mailer.Message{To: "Budi <budi@contoh.id>", Subject: "SUBJEK-RAHASIA", Text: "PASSWORD-RAHASIA"}
}

func TestDeliver_Sent(t *testing.T) {
	f := &mailertest.Fake{}
	if got := Deliver(context.Background(), f, msg(), nil); got != base.DeliverySent {
		t.Fatal(got)
	}
	if m, ok := f.Last(); !ok || m.Text != "PASSWORD-RAHASIA" {
		t.Fatal("pesan harus terkirim")
	}
}

func TestDeliver_NotConfigured(t *testing.T) {
	if got := Deliver(context.Background(), &mailertest.Fake{NotConfigured: true}, msg(), nil); got != base.DeliveryNotConfigured {
		t.Fatal(got)
	}
	if got := Deliver(context.Background(), nil, msg(), nil); got != base.DeliveryNotConfigured {
		t.Fatal(got)
	}
}

func TestDeliver_FailedLogsWithoutSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	f := &mailertest.Fake{Err: errors.New("mailer: smtp rcpt: 550 no such user")}
	if got := Deliver(context.Background(), f, msg(), log); got != base.DeliveryFailed {
		t.Fatal(got)
	}
	out := buf.String()
	if !strings.Contains(out, "invitation email failed") || !strings.Contains(out, "contoh.id") {
		t.Fatalf("log: %s", out)
	}
	for _, secret := range []string{"PASSWORD-RAHASIA", "SUBJEK-RAHASIA", "budi@"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log memuat %q: %s", secret, out)
		}
	}
}

func TestMessage(t *testing.T) {
	m := Message("a@b.id", base.EmailContent{Subject: "s", Text: "t", HTML: "h"})
	if m.To != "a@b.id" || m.Subject != "s" || m.Text != "t" || m.HTML != "h" {
		t.Fatalf("%+v", m)
	}
}
