package touchlastused

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
)

// Throttle penulisan last_used_at: paling sering sekali per menit per key.
// Nilai 1 menit ditulis literal di SQL (K7, tiket T04) sehingga benar pada multi-instance.
// Batas inklusif: tepat 1 menit setelah update sebelumnya boleh menulis lagi.
const touchSQL = `UPDATE api_keys SET last_used_at = $2
WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $2::timestamptz - interval '1 minute')`

// Repo adalah satu-satunya yang menyentuh SQL.
type Repo struct{ q db.Querier }

func newRepo(s *store.Store) *Repo { return &Repo{q: s.Querier()} }

// Touch mengembalikan jumlah baris yang diubah (1 = tertulis, 0 = di-throttle / key tidak ada).
func (r *Repo) Touch(ctx context.Context, id string, now time.Time) (int64, error) {
	res, err := r.q.ExecContext(ctx, touchSQL, id, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("touchlastused: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("touchlastused: rows affected: %w", err)
	}
	return n, nil
}
