// Package mailertest menyediakan Mailer palsu in-memory untuk tes domain.
package mailertest

import (
	"context"
	"sync"

	"grimoire/internal/infra/mailer"
)

// Fake menyimpan pesan yang "terkirim". Err diset -> Send gagal; NotConfigured -> ErrNotConfigured.
type Fake struct {
	Sent          []mailer.Message
	Err           error
	NotConfigured bool
	mu            sync.Mutex
}

var _ mailer.Mailer = (*Fake)(nil)

// Send menyimpan m bila tidak ada error yang diset.
func (f *Fake) Send(_ context.Context, m mailer.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.NotConfigured {
		return mailer.ErrNotConfigured
	}
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, m)
	return nil
}

// Configured false bila NotConfigured.
func (f *Fake) Configured() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.NotConfigured
}

// Last mengembalikan pesan terakhir yang terkirim.
func (f *Fake) Last() (mailer.Message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Sent) == 0 {
		return mailer.Message{}, false
	}
	return f.Sent[len(f.Sent)-1], true
}
