package secretbox

import (
	"bytes"
	"errors"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestRoundTrip(t *testing.T) {
	bx, _ := New(key(1))
	blob, err := bx.Seal([]byte("secret"), []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := bx.Open(blob, []byte("aad"))
	if err != nil || string(got) != "secret" {
		t.Fatal(got, err)
	}
}

func TestFailures(t *testing.T) {
	a, _ := New(key(1))
	b, _ := New(key(2))
	blob, _ := a.Seal([]byte("secret"), []byte("aad"))
	if _, err := b.Open(blob, []byte("aad")); !errors.Is(err, ErrDecrypt) {
		t.Error("wrong key")
	}
	if _, err := a.Open(blob, []byte("other")); !errors.Is(err, ErrDecrypt) {
		t.Error("wrong aad")
	}
	if _, err := a.Open(blob[:10], []byte("aad")); !errors.Is(err, ErrDecrypt) {
		t.Error("short")
	}
	if _, err := a.Open(nil, nil); !errors.Is(err, ErrDecrypt) {
		t.Error("nil")
	}
	if _, err := a.Open(blob[:len(blob)-1], []byte("aad")); !errors.Is(err, ErrDecrypt) {
		t.Error("truncated")
	}
}

func TestNonceUnique(t *testing.T) {
	a, _ := New(key(1))
	x, _ := a.Seal([]byte("same"), nil)
	y, _ := a.Seal([]byte("same"), nil)
	if bytes.Equal(x, y) {
		t.Fatal("nonce reused")
	}
}

func TestKeySize(t *testing.T) {
	for _, n := range []int{0, 31, 33} {
		if _, err := New(make([]byte, n)); !errors.Is(err, ErrKeySize) {
			t.Errorf("len %d", n)
		}
	}
}
