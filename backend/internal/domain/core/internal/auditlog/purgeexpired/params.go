package purgeexpired

import "time"

const (
	BatchSize = 5000
	Pause     = 100 * time.Millisecond
)

// Options mengatur loop purge. Init selalu mengisi semua field secara eksplisit.
type Options struct {
	Retention int           // hari, dari Config.AuditRetentionDays; harus > 0
	BatchSize int           // <= 0 -> BatchSize
	Pause     time.Duration // < 0 -> 0; 0 = tanpa jeda (tes)
}
