// Package ingest adalah satu jalur tulis batch file knowledge: dipakai upload multipart dan
// sync Git (K7). Rule (ClassifyPart, PlanUpload) tetap di package base; paket ini hanya
// mengorkestrasi urutan di dalam transaksi milik pemanggil.
package ingest

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

// Tx adalah akses data yang dibutuhkan, sudah terikat satu transaksi.
type Tx interface {
	LockProject(ctx context.Context, projectID string) error
	CountFiles(ctx context.Context, projectID string) (int, error)
	ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
	InsertFiles(ctx context.Context, files []base.File) error
	UpdateFiles(ctx context.Context, files []base.File) error
}

// Apply WAJIB dipanggil DI DALAM transaksi pemanggil. Urutan: lock project, hitung file, ambil hash
// yang ada, rencanakan (base.PlanUpload), lalu tulis. Mengembalikan keputusan terencana dan jumlah
// byte yang ditulis. incoming kosong -> tidak melakukan apa pun.
func Apply(ctx context.Context, tx Tx, projectID string, actor base.Actor, incoming []base.Incoming, maxProjectFiles int) ([]base.Decision, int, error) {
	if len(incoming) == 0 {
		return nil, 0, nil
	}
	if err := tx.LockProject(ctx, projectID); err != nil {
		return nil, 0, fmt.Errorf("lock project: %w", err)
	}
	count, err := tx.CountFiles(ctx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("count files: %w", err)
	}
	paths := make([]string, 0, len(incoming))
	for _, f := range incoming {
		paths = append(paths, f.Path)
	}
	existing, err := tx.ExistingHashes(ctx, projectID, paths)
	if err != nil {
		return nil, 0, fmt.Errorf("existing hashes: %w", err)
	}
	planned := base.PlanUpload(incoming, existing, count, maxProjectFiles)

	byIndex := make(map[int]base.Incoming, len(incoming))
	for _, f := range incoming {
		byIndex[f.Index] = f
	}
	var created, updated []base.File
	bytesWritten := 0
	for _, d := range planned {
		f := byIndex[d.Index]
		switch d.Status {
		case base.StatusCreated:
			nf := base.NewFile(projectID, f.Path, f.Content, actor)
			created = append(created, nf)
			bytesWritten += nf.SizeBytes
		case base.StatusUpdated:
			nf := base.NewFile(projectID, f.Path, f.Content, actor)
			updated = append(updated, nf)
			bytesWritten += nf.SizeBytes
		}
	}
	if len(created) > 0 {
		if err := tx.InsertFiles(ctx, created); err != nil {
			return nil, 0, fmt.Errorf("insert files: %w", err)
		}
	}
	if len(updated) > 0 {
		if err := tx.UpdateFiles(ctx, updated); err != nil {
			return nil, 0, fmt.Errorf("update files: %w", err)
		}
	}
	return planned, bytesWritten, nil
}
