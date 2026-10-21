package base

import "time"

// Clock memberi waktu sekarang; diinjeksi agar test mengontrol waktu.
type Clock func() time.Time

// SystemClock adalah Clock produksi (UTC).
func SystemClock() time.Time { return time.Now().UTC() }
