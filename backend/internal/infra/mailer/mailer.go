// Package mailer mengirim email lewat driver yang dipilih dari konfigurasi (smtp, log,
// atau noop bila tidak dikonfigurasi). Paket ini generik: tidak tahu soal undangan atau
// bisnis apa pun (rules.txt 4.6).
package mailer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"

	"grimoire/internal/infra/config"
)

// Message adalah satu email ke satu penerima (tanpa CC/BCC by design).
type Message struct{ To, Subject, Text, HTML string }

// Mailer mengirim email. Implementasi aman dipakai bersamaan.
type Mailer interface {
	// Send mengirim satu pesan, satu percobaan tanpa retry.
	Send(ctx context.Context, m Message) error
	// Configured false bila tidak ada driver yang mengirim sungguhan (noop).
	Configured() bool
}

// ErrNotConfigured dikembalikan driver noop.
var ErrNotConfigured = errors.New("mailer: tidak terkonfigurasi")

// New memilih driver dari cfg.MailDriver: "smtp", "log", atau kosong (noop).
func New(cfg config.Config, log *slog.Logger) (Mailer, error) {
	if log == nil {
		log = slog.Default()
	}
	switch cfg.MailDriver {
	case "":
		return noop{}, nil
	case "log":
		return newLog(log), nil
	case "smtp":
		if cfg.SMTPTLS == "none" && cfg.AppEnv == "production" {
			log.Warn("mailer: SMTP_TLS=none di produksi; email dan kredensial SMTP terkirim tanpa enkripsi")
		}
		return newSMTP(cfg)
	default:
		return nil, fmt.Errorf("mailer: driver %q tidak dikenal", cfg.MailDriver)
	}
}

// validate menolak pesan yang berpotensi header injection atau kosong.
func validate(m Message) error {
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("mailer: To/Subject tidak boleh memuat CR/LF")
	}
	if a, err := mail.ParseAddress(m.To); err != nil || a.Address == "" {
		return errors.New("mailer: alamat penerima tidak valid")
	}
	if m.Text == "" && m.HTML == "" {
		return errors.New("mailer: pesan harus punya Text atau HTML")
	}
	return nil
}
