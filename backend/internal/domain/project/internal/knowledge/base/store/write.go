package store

import (
	"context"
	"encoding/hex"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

// writeChunkSize membatasi jumlah baris per statement unnest.
const writeChunkSize = 100

const insertFilesSQL = `INSERT INTO knowledge_files
  (project_id, path, title, content, content_hash, size_bytes,
   created_by, created_by_label, updated_by, updated_by_label)
SELECT $1, u.path, u.title, u.content, decode(u.hash, 'hex'), u.size,
       $2::uuid, $3, $2::uuid, $3
FROM unnest($4::text[], $5::text[], $6::text[], $7::text[], $8::int[])
     AS u(path, title, content, hash, size)`

const updateFilesSQL = `UPDATE knowledge_files f
SET title = u.title, content = u.content, content_hash = decode(u.hash, 'hex'),
    size_bytes = u.size, updated_at = now(), updated_by = $2::uuid, updated_by_label = $3
FROM unnest($4::text[], $5::text[], $6::text[], $7::text[], $8::int[])
     AS u(path, title, content, hash, size)
WHERE f.project_id = $1 AND f.path = u.path`

type hashRow struct {
	Path string `db:"path"`
	Hash []byte `db:"content_hash"`
}

// ExistingHashes mengembalikan content_hash file yang sudah ada untuk paths (key = path).
func (s *Store) ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if len(paths) == 0 || !ValidUUID(projectID) {
		return out, nil
	}
	var rows []hashRow
	if err := s.q.SelectContext(ctx, &rows,
		`SELECT path, content_hash FROM knowledge_files WHERE project_id = $1 AND path = ANY($2)`,
		projectID, paths); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Path] = row.Hash
	}
	return out, nil
}

type writeColumns struct {
	paths, titles, contents, hashes []string
	sizes                           []int32
}

func toWriteColumns(files []base.File) writeColumns {
	var c writeColumns
	for _, f := range files {
		c.paths = append(c.paths, f.Path)
		c.titles = append(c.titles, f.Title)
		c.contents = append(c.contents, f.Content)
		c.hashes = append(c.hashes, hex.EncodeToString(f.Hash))
		c.sizes = append(c.sizes, int32(f.SizeBytes))
	}
	return c
}

func (s *Store) writeFiles(ctx context.Context, query, op string, files []base.File) error {
	for start := 0; start < len(files); start += writeChunkSize {
		end := min(start+writeChunkSize, len(files))
		chunk := files[start:end]
		c := toWriteColumns(chunk)
		res, err := s.q.ExecContext(ctx, query, chunk[0].ProjectID, chunk[0].UpdatedBy, chunk[0].UpdatedByLabel,
			c.paths, c.titles, c.contents, c.hashes, c.sizes)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if int(n) != len(chunk) {
			return fmt.Errorf("store: %s files: rows affected %d, want %d", op, n, len(chunk))
		}
	}
	return nil
}

// InsertFiles menyisipkan file baru per chunk. Semua file harus satu project dan satu aktor.
func (s *Store) InsertFiles(ctx context.Context, files []base.File) error {
	return s.writeFiles(ctx, insertFilesSQL, "insert", files)
}

// UpdateFiles memperbarui file yang sudah ada per chunk.
func (s *Store) UpdateFiles(ctx context.Context, files []base.File) error {
	return s.writeFiles(ctx, updateFilesSQL, "update", files)
}
