package mailer

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLogMailer_PrintsEmail(t *testing.T) {
	var buf bytes.Buffer
	m := newLog(slog.New(slog.NewTextHandler(&buf, nil)))
	if !m.Configured() {
		t.Fatal("log harus Configured")
	}
	if err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "Halo", Text: "isi-pesan"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, w := range []string{"a@b.id", "Halo", "isi-pesan"} {
		if !strings.Contains(out, w) {
			t.Errorf("log tidak memuat %q: %s", w, out)
		}
	}
	if err := m.Send(context.Background(), Message{To: "x", Subject: "s", Text: "t"}); err == nil {
		t.Fatal("pesan invalid harus ditolak")
	}
}
