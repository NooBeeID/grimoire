// Package secretbox mengenkripsi rahasia dengan AES-256-GCM. Generik: format AAD
// ditentukan pemanggil.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrDecrypt = errors.New("secretbox: decrypt failed")
	ErrKeySize = errors.New("secretbox: key must be 32 bytes")
)

// Box membungkus AEAD AES-256-GCM.
type Box struct{ aead cipher.AEAD }

// New membuat Box; key wajib 32 byte.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, ErrKeySize
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	return &Box{aead: g}, nil
}

// Seal mengembalikan nonce(12) || ciphertext+tag.
func (b *Box) Seal(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, aad), nil
}

// Open membuka blob hasil Seal; kegagalan apa pun -> ErrDecrypt.
func (b *Box) Open(blob, aad []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(blob) < ns+b.aead.Overhead() {
		return nil, ErrDecrypt
	}
	plain, err := b.aead.Open(nil, blob[:ns], blob[ns:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}
