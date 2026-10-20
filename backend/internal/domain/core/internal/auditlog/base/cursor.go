package base

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cursor menandai posisi keyset (occurred_at, id) pada urutan DESC.
type Cursor struct {
	OccurredAt time.Time
	ID         string
}

// EncodeCursor menghasilkan token opaque base64url dari "<RFC3339Nano UTC>|<uuid>".
func EncodeCursor(c Cursor) string {
	raw := c.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor mengurai token cursor. Token rusak -> error ErrInvalid (field cursor).
func DecodeCursor(s string) (Cursor, error) {
	fail := func() (Cursor, error) {
		return Cursor{}, fieldErr("cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return fail()
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return fail()
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return fail()
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return fail()
	}
	return Cursor{OccurredAt: t.UTC(), ID: u.String()}, nil
}

// Cursor mengembalikan posisi keyset entri ini.
func (e Entry) Cursor() Cursor {
	return Cursor{OccurredAt: e.OccurredAt.UTC(), ID: e.ID}
}
