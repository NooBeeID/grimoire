package mailer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/config"
)

func TestNew_PicksDriver(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	m, err := New(config.Config{}, log)
	if err != nil || m.Configured() {
		t.Fatalf("driver kosong harus noop: %v %v", m, err)
	}
	if err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "s", Text: "t"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("noop harus ErrNotConfigured, got %v", err)
	}
	m, err = New(config.Config{MailDriver: "log"}, log)
	if err != nil || !m.Configured() {
		t.Fatalf("log: %v %v", m, err)
	}
	m, err = New(config.Config{MailDriver: "smtp", SMTPHost: "h", SMTPPort: 25, SMTPTLS: "none", MailFrom: "a@b.id"}, log)
	if err != nil || !m.Configured() {
		t.Fatalf("smtp: %v %v", m, err)
	}
	if _, err := New(config.Config{MailDriver: "x"}, log); err == nil {
		t.Fatal("driver tak dikenal harus error")
	}
	if _, err := New(config.Config{MailDriver: "smtp", SMTPHost: "h", MailFrom: "bukan"}, log); err == nil {
		t.Fatal("MAIL_FROM invalid harus error")
	}
}

func TestNew_WarnsPlainTLSInProduction(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	_, err := New(config.Config{AppEnv: "production", MailDriver: "smtp", SMTPHost: "h", SMTPTLS: "none", MailFrom: "a@b.id"}, log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "SMTP_TLS=none") {
		t.Fatalf("peringatan tidak ada: %s", buf.String())
	}
}

func TestNew_RejectsAuthWithoutTLSOnRemoteHost(t *testing.T) {
	_, err := New(config.Config{MailDriver: "smtp", SMTPHost: "mail.example.com", SMTPTLS: "none", SMTPUsername: "u", MailFrom: "a@b.id"}, nil)
	if err == nil {
		t.Fatal("harus error")
	}
}

func TestValidate(t *testing.T) {
	ok := Message{To: "a@b.id", Subject: "s", Text: "t"}
	if err := validate(ok); err != nil {
		t.Fatal(err)
	}
	bad := []Message{
		{To: "a@b.id\r\nBcc: x@y.id", Subject: "s", Text: "t"},
		{To: "a@b.id", Subject: "s\r\nBcc: x@y.id", Text: "t"},
		{To: "a@b.id", Subject: "s\nX: y", Text: "t"},
		{To: "bukan-email", Subject: "s", Text: "t"},
		{To: "", Subject: "s", Text: "t"},
		{To: "a@b.id", Subject: "s"},
	}
	for i, m := range bad {
		if err := validate(m); err == nil {
			t.Errorf("kasus %d harus ditolak", i)
		}
	}
	if err := validate(Message{To: "a@b.id", Subject: "s", HTML: "<p>x</p>"}); err != nil {
		t.Fatalf("HTML saja harus lolos: %v", err)
	}
}
