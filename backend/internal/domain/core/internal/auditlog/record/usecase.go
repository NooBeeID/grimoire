// Package record adalah feature core.audit.record: menyimpan satu entri audit.
package record

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah sisi persistensi yang dibutuhkan usecase.
type Repository interface {
	Insert(ctx context.Context, e base.Entry) error
}

// insertTimeout adalah batas waktu satu INSERT (variabel agar bisa di-override tes).
var insertTimeout = 2 * time.Second

// Usecase mengorkestrasi penyimpanan entri audit (fail-open di sisi consumer).
type Usecase struct {
	repo  Repository
	now   func() time.Time
	newID func() string
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, now func() time.Time, newID func() string) *Usecase {
	return &Usecase{repo: repo, now: now, newID: newID}
}

// Execute memvalidasi dan menyimpan satu entri. Tidak pernah panic dan tidak me-log;
// error dikembalikan ke pemanggil.
func (u *Usecase) Execute(ctx context.Context, req record.Request) (record.Response, error) {
	if req.TraceID == "" {
		req.TraceID = reqctx.TraceID(ctx)
	}
	e, err := base.NewEntry(req, u.now(), u.newID)
	if err != nil {
		return record.Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, insertTimeout)
	defer cancel()
	if err := u.repo.Insert(ctx, e); err != nil {
		return record.Response{}, fmt.Errorf("audit record: %w", err)
	}
	return record.Response{Recorded: true}, nil
}
