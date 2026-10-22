package base

import (
	"sort"
	"strings"
)

// TreeEntry adalah satu file di tree repo; tipe lokal (diisi usecase dari klien Git) agar base
// tidak meng-import infra.
type TreeEntry struct {
	Path    string
	Size    int64 // -1 bila tidak diketahui
	BlobSHA string
}

// Candidate adalah file yang akan di-fetch dan ditulis.
type Candidate struct {
	RepoPath string // path lengkap di repo
	RelPath  string // RepoPath tanpa prefix folder
	SHA      string
	Size     int64
}

// SelectFiles memilih entri di bawah folder (rekursif; "" = semua). Entri dengan ekstensi tidak
// didukung masuk skipped (reason dari Kind.SkipReason). Folder tidak kosong tanpa satu pun entri
// di bawahnya -> ErrFolderNotFound. Hasil terurut RepoPath naik. Path repo tidak dinormalisasi.
func SelectFiles(entries []TreeEntry, folder string, k Kind) (cands []Candidate, skipped []ReportItem, err error) {
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}
	found := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Path, prefix) || e.Path == prefix {
			continue
		}
		found = true
		if !k.AllowedExt(e.Path) {
			skipped = append(skipped, ReportItem{RepoPath: e.Path, Status: ItemSkipped, ReasonCode: k.SkipReason(), Issues: []Issue{}})
			continue
		}
		cands = append(cands, Candidate{RepoPath: e.Path, RelPath: strings.TrimPrefix(e.Path, prefix), SHA: e.BlobSHA, Size: e.Size})
	}
	if folder != "" && !found {
		return nil, nil, ErrFolderNotFound
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].RepoPath < cands[j].RepoPath })
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].RepoPath < skipped[j].RepoPath })
	return cands, skipped, nil
}
