package base

import (
	"sort"
	"strings"
	"time"
)

type FolderNode struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	FileCount int    `json:"file_count"` // rekursif
}

type FileNode struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SizeBytes int       `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Level struct {
	Prefix     string       `json:"prefix"`
	Folders    []FolderNode `json:"folders"` // tidak pernah nil (JSON [])
	Files      []FileNode   `json:"files"`
	TotalFiles int          `json:"total_files"` // file rekursif di bawah prefix
}

// BuildLevel menyusun satu level pohon dari semua meta di bawah prefix (rekursif). Meta yang tidak
// berawalan prefix diabaikan. Folder dan file diurutkan menurut Name (byte order).
func BuildLevel(prefix string, metas []Meta) Level {
	lv := Level{Prefix: prefix, Folders: []FolderNode{}, Files: []FileNode{}}
	head := ""
	if prefix != "" {
		head = prefix + "/"
	}
	counts := map[string]int{}
	for _, m := range metas {
		if !strings.HasPrefix(m.Path, head) {
			continue
		}
		rest := m.Path[len(head):]
		if rest == "" {
			continue
		}
		lv.TotalFiles++
		if i := strings.Index(rest, "/"); i >= 0 {
			counts[rest[:i]]++
			continue
		}
		lv.Files = append(lv.Files, FileNode{Name: rest, Path: m.Path, SizeBytes: m.SizeBytes, UpdatedAt: m.UpdatedAt})
	}
	for name, n := range counts {
		lv.Folders = append(lv.Folders, FolderNode{Name: name, Path: head + name, FileCount: n})
	}
	sort.Slice(lv.Folders, func(i, j int) bool { return lv.Folders[i].Name < lv.Folders[j].Name })
	sort.Slice(lv.Files, func(i, j int) bool { return lv.Files[i].Name < lv.Files[j].Name })
	return lv
}
