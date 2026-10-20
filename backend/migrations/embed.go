// Package migrations meng-embed file migrasi SQL. Tidak berisi logika.
package migrations

import "embed"

// FS berisi seluruh file migrasi. all: agar .keep ikut di-embed dan folder
// kosong tidak menggagalkan build.
//
//go:embed all:core all:project
var FS embed.FS
