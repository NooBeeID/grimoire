// Package delivery mengirim email undangan lewat infra/mailer dan memetakan hasilnya ke
// status pengiriman. Satu-satunya bagian base/ yang menyentuh infra (pola base/audit).
package delivery

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/mailer"
)

// Message memetakan EmailContent ke pesan mailer untuk penerima to.
func Message(to string, c base.EmailContent) mailer.Message {
	return mailer.Message{To: to, Subject: c.Subject, Text: c.Text, HTML: c.HTML}
}

// Deliver mengirim m lewat ml dan mengembalikan status pengiriman
// (base.DeliverySent, DeliveryNotConfigured, atau DeliveryFailed).
// Tidak pernah me-log body, subject, atau password; hanya domain penerima dan error mailer
// (yang sudah aman dari isi pesan). Tidak membuat ctx baru: pemanggil sebaiknya memakai
// context.WithoutCancel(ctx) + timeout MailTimeout agar klien yang memutus koneksi setelah
// commit tidak membatalkan pengiriman.
func Deliver(ctx context.Context, ml mailer.Mailer, m mailer.Message, log *slog.Logger) string {
	if log == nil {
		log = slog.Default()
	}
	if ml == nil {
		log.InfoContext(ctx, "invitation email skipped: mailer not configured")
		return base.DeliveryNotConfigured
	}
	err := ml.Send(ctx, m)
	switch {
	case err == nil:
		return base.DeliverySent
	case errors.Is(err, mailer.ErrNotConfigured):
		log.InfoContext(ctx, "invitation email skipped: mailer not configured")
		return base.DeliveryNotConfigured
	default:
		log.ErrorContext(ctx, "invitation email failed", "to_domain", domainOf(m.To), "err", err)
		return base.DeliveryFailed
	}
}

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.TrimRight(addr[i+1:], "> ")
	}
	return ""
}
