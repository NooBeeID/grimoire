package base

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

func TestCursor_RoundTrip(t *testing.T) {
	c := Cursor{OccurredAt: time.Date(2026, 10, 19, 8, 14, 20, 123456789, time.UTC), ID: uid3}
	got, err := DecodeCursor(EncodeCursor(c))
	if err != nil {
		t.Fatal(err)
	}
	if !got.OccurredAt.Equal(c.OccurredAt) || got.ID != c.ID {
		t.Fatalf("got %+v", got)
	}
}

func TestCursor_Invalid(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, s := range []string{
		"!!!",
		enc("no-separator"),
		enc("not-a-time|" + uid3),
		enc("2026-10-19T08:14:20Z|not-a-uuid"),
	} {
		_, err := DecodeCursor(s)
		if !errors.Is(err, errs.ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", s, err)
		}
	}
}
