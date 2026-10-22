package gitprovider

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized   = errors.New("gitprovider: unauthorized")
	ErrForbidden      = errors.New("gitprovider: forbidden")
	ErrNotFound       = errors.New("gitprovider: not found")
	ErrTreeTooLarge   = errors.New("gitprovider: tree too large")
	ErrUpstream       = errors.New("gitprovider: upstream error")
	ErrHostNotAllowed = errors.New("gitprovider: host not allowed")
	ErrRateLimited    = errors.New("gitprovider: rate limited")
)

// RateLimitedError dikembalikan saat provider membatasi permintaan. RetryAfter 0 = tidak diketahui.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return ErrRateLimited.Error() }

// Is membuat errors.Is(err, ErrRateLimited) benar.
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }
