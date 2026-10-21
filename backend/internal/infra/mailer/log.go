package mailer

import (
	"context"
	"log/slog"
)

// logMailer mencetak email utuh ke log. Hanya untuk dev/tes (dicegah di produksi oleh config).
type logMailer struct{ log *slog.Logger }

func newLog(log *slog.Logger) Mailer { return &logMailer{log: log} }

func (l *logMailer) Send(ctx context.Context, m Message) error {
	if err := validate(m); err != nil {
		return err
	}
	l.log.InfoContext(ctx, "mailer(log): email", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

func (l *logMailer) Configured() bool { return true }
