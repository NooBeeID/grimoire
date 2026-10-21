package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/infra/config"
)

// fakeSMTP adalah server SMTP minimal (tanpa STARTTLS).
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	cmds     []string
	data     string
	rcptCode string // balasan RCPT; default 250
	hang     bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, rcptCode: "250 ok"}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(s string) { c.Write([]byte(s + "\r\n")) }
	w("220 fake ready")
	if f.hang {
		time.Sleep(5 * time.Second)
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.mu.Lock()
		f.cmds = append(f.cmds, line)
		f.mu.Unlock()
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"):
			w("250-fake\r\n250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH"):
			w("235 ok")
		case strings.HasPrefix(up, "MAIL"):
			w("250 ok")
		case strings.HasPrefix(up, "RCPT"):
			w(f.rcptCode)
		case up == "DATA":
			w("354 go")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			f.mu.Lock()
			f.data = sb.String()
			f.mu.Unlock()
			w("250 queued")
		case up == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func (f *fakeSMTP) hasCmd(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if strings.HasPrefix(strings.ToUpper(c), prefix) {
			return true
		}
	}
	return false
}

func cfgFor(f *fakeSMTP, mut func(*config.Config)) config.Config {
	c := config.Config{MailDriver: "smtp", SMTPHost: "127.0.0.1", SMTPPort: f.port(), SMTPTLS: "none",
		MailFrom: "Grimoire <no-reply@grimoire.local>", MailTimeout: 2 * time.Second}
	if mut != nil {
		mut(&c)
	}
	return c
}

func TestSMTP_SendsMultipartUTF8(t *testing.T) {
	f := newFakeSMTP(t)
	m, err := New(cfgFor(f, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = m.Send(context.Background(), Message{To: "Budi <budi@x.id>", Subject: "Undangan é", Text: "halo dunia é", HTML: "<p>halo é</p>"})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	data := f.data
	f.mu.Unlock()
	for _, w := range []string{"multipart/alternative", "text/plain; charset=UTF-8", "text/html; charset=UTF-8",
		"quoted-printable", "From: ", "To: ", "Message-ID: <", "MIME-Version: 1.0", "=?utf-8?q?"} {
		if !strings.Contains(data, w) {
			t.Errorf("data tidak memuat %q:\n%s", w, data)
		}
	}
	if strings.Count(data, "Content-Type: text/") != 2 {
		t.Errorf("harus dua bagian:\n%s", data)
	}
	if f.hasCmd("AUTH") {
		t.Error("AUTH tidak boleh dikirim tanpa username")
	}
}

func TestSMTP_AuthOnlyWithUsername(t *testing.T) {
	f := newFakeSMTP(t)
	m, err := New(cfgFor(f, func(c *config.Config) { c.SMTPUsername = "u"; c.SMTPPassword = "p" }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if !f.hasCmd("AUTH PLAIN") {
		t.Fatal("AUTH PLAIN harus dikirim")
	}
}

func TestSMTP_RcptRejectedWrapsStageWithoutBody(t *testing.T) {
	f := newFakeSMTP(t)
	f.rcptCode = "550 no such user"
	m, _ := New(cfgFor(f, nil), nil)
	err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "RAHASIA-SUBJEK", Text: "RAHASIA-BODY"})
	if err == nil {
		t.Fatal("harus error")
	}
	if !strings.Contains(err.Error(), "smtp rcpt") {
		t.Fatalf("harus menyebut tahap rcpt: %v", err)
	}
	if strings.Contains(err.Error(), "RAHASIA") {
		t.Fatalf("error memuat body/subject: %v", err)
	}
}

func TestSMTP_StartTLSRequired(t *testing.T) {
	f := newFakeSMTP(t)
	m, _ := New(cfgFor(f, func(c *config.Config) { c.SMTPTLS = "starttls" }), nil)
	err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "smtp starttls") {
		t.Fatalf("harus gagal di starttls: %v", err)
	}
	if f.hasCmd("MAIL") {
		t.Fatal("tidak boleh lanjut tanpa TLS")
	}
}

func TestSMTP_ContextDeadline(t *testing.T) {
	f := newFakeSMTP(t)
	f.hang = true
	m, _ := New(cfgFor(f, func(c *config.Config) { c.MailTimeout = 30 * time.Second }), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := m.Send(ctx, Message{To: "a@b.id", Subject: "s", Text: "t"})
	if err == nil {
		t.Fatal("harus error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("terlalu lama: %s", time.Since(start))
	}
}

func TestSMTP_DialFailure(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	c := config.Config{MailDriver: "smtp", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPTLS: "none",
		MailFrom: "a@b.id", MailTimeout: time.Second}
	m, _ := New(c, nil)
	err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "smtp dial") {
		t.Fatalf("got %v", err)
	}
}

func TestSMTP_HeaderInjectionRejectedBeforeDial(t *testing.T) {
	f := newFakeSMTP(t)
	m, _ := New(cfgFor(f, nil), nil)
	if err := m.Send(context.Background(), Message{To: "a@b.id", Subject: "s\r\nBcc: x@y.id", Text: "t"}); err == nil {
		t.Fatal("harus ditolak")
	}
	if f.hasCmd("EHLO") {
		t.Fatal("tidak boleh dial")
	}
}
