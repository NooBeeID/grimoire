package base

import (
	"strings"

	"grimoire/lib/common/secret"
)

const (
	minTokenBytes = 8
	maxTokenBytes = 512
)

// ValidateToken: trim; 8-512 byte; hanya ASCII cetak 0x21..0x7E. Pesan error tidak memuat isi token.
func ValidateToken(raw string) (secret.String, error) {
	t := strings.TrimSpace(raw)
	if len(t) < minTokenBytes || len(t) > maxTokenBytes {
		return secret.String{}, ErrTokenFormat
	}
	for i := 0; i < len(t); i++ {
		if t[i] < 0x21 || t[i] > 0x7E {
			return secret.String{}, ErrTokenFormat
		}
	}
	return secret.New(t), nil
}
