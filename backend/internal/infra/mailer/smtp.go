package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/infra/config"
)

type smtpMailer struct {
	host, user, pass, tlsMode string
	port                      int
	from                      *mail.Address
	timeout                   time.Duration
}

func newSMTP(cfg config.Config) (Mailer, error) {
	from, err := mail.ParseAddress(cfg.MailFrom)
	if err != nil {
		return nil, errors.New("mailer: MAIL_FROM tidak valid")
	}
	if cfg.SMTPHost == "" {
		return nil, errors.New("mailer: SMTP_HOST kosong")
	}
	if cfg.SMTPUsername != "" && cfg.SMTPTLS == "none" && !isLocalHost(cfg.SMTPHost) {
		return nil, errors.New("mailer: SMTP_USERNAME dengan SMTP_TLS=none hanya diizinkan untuk host lokal")
	}
	t := cfg.MailTimeout
	if t <= 0 {
		t = 10 * time.Second
	}
	return &smtpMailer{host: cfg.SMTPHost, port: cfg.SMTPPort, user: cfg.SMTPUsername, pass: cfg.SMTPPassword,
		tlsMode: cfg.SMTPTLS, from: from, timeout: t}, nil
}

func isLocalHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *smtpMailer) Configured() bool { return true }

func stageErr(stage string, err error) error {
	return fmt.Errorf("mailer: smtp %s: %w", stage, err)
}

func (s *smtpMailer) Send(ctx context.Context, m Message) error {
	if err := validate(m); err != nil {
		return err
	}
	to, _ := mail.ParseAddress(m.To)
	body, err := s.build(m, to)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(s.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{Timeout: s.timeout, Deadline: deadline}
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	if s.tlsMode == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return stageErr("dial", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	// Batalkan I/O bila ctx dibatalkan di tengah jalan.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return stageErr("dial", err)
	}
	defer c.Close()

	if s.tlsMode == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return stageErr("starttls", errors.New("server tidak mendukung STARTTLS"))
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return stageErr("starttls", err)
		}
	}
	if s.user != "" {
		if err := c.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
			return stageErr("auth", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return stageErr("mail", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return stageErr("rcpt", err)
	}
	w, err := c.Data()
	if err != nil {
		return stageErr("data", err)
	}
	if _, err := w.Write(body); err != nil {
		return stageErr("data", err)
	}
	if err := w.Close(); err != nil {
		return stageErr("data", err)
	}
	if err := c.Quit(); err != nil {
		return stageErr("quit", err)
	}
	return nil
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func qpPart(contentType, content string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(content)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	b.WriteString("\r\n")
	return b.Bytes(), nil
}

// build menyusun pesan MIME multipart/alternative.
func (s *smtpMailer) build(m Message, to *mail.Address) ([]byte, error) {
	boundary, err := randHex(16)
	if err != nil {
		return nil, err
	}
	rid, err := randHex(12)
	if err != nil {
		return nil, err
	}
	domain := "localhost"
	if i := strings.LastIndex(s.from.Address, "@"); i >= 0 {
		domain = s.from.Address[i+1:]
	}
	text, html := m.Text, m.HTML
	if text == "" {
		text = " "
	}
	var b bytes.Buffer
	b.WriteString("From: " + s.from.String() + "\r\n")
	b.WriteString("To: " + to.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + rid + "@" + domain + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
	tp, err := qpPart("text/plain", text)
	if err != nil {
		return nil, err
	}
	b.WriteString("--" + boundary + "\r\n")
	b.Write(tp)
	if html != "" {
		hp, err := qpPart("text/html", html)
		if err != nil {
			return nil, err
		}
		b.WriteString("--" + boundary + "\r\n")
		b.Write(hp)
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}
