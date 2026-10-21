package base

import (
	"slices"
	"strings"
)

// SupportedVersions versi protokol MCP yang didukung, terbaru dulu.
var SupportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

const (
	DefaultHeaderVersion  = "2025-03-26"
	ServerName            = "grimoire"
	HeaderProtocolVersion = "MCP-Protocol-Version"
)

// Negotiate: persis salah satu SupportedVersions -> (requested, true).
func Negotiate(requested string) (string, bool) {
	if slices.Contains(SupportedVersions, requested) {
		return requested, true
	}
	return "", false
}

// KnownHeaderVersion memeriksa header MCP-Protocol-Version; kosong -> default.
func KnownHeaderVersion(h string) (string, bool) {
	if h == "" {
		return DefaultHeaderVersion, true
	}
	return Negotiate(h)
}

// Instructions membuat teks panduan untuk klien MCP.
func Instructions(projectName string, toolNames []string) string {
	name := projectName
	if name == "" {
		name = "project ini"
	}
	if len(toolNames) == 0 {
		return "Server pengetahuan untuk " + name + ". Belum ada tool yang terdaftar."
	}
	has := func(n string) bool { return slices.Contains(toolNames, n) }
	var b strings.Builder
	b.WriteString("Server pengetahuan untuk " + name + ". Semua tool hanya-baca. Tool tersedia: " + strings.Join(toolNames, ", ") + ".")
	var steps []string
	if has("list_files") {
		steps = append(steps, "list_files untuk melihat isi")
	}
	if has("read_file") {
		steps = append(steps, "read_file untuk membaca (gunakan offset/limit untuk file besar)")
	}
	if has("search_knowledge") {
		steps = append(steps, "search_knowledge untuk mencari")
	}
	if has("list_api_specs") {
		steps = append(steps, "list_api_specs untuk melihat spesifikasi API")
	}
	if has("list_api_endpoints") {
		steps = append(steps, "list_api_endpoints untuk melihat endpoint")
	}
	if has("get_api_endpoint") {
		steps = append(steps, "get_api_endpoint untuk detail endpoint")
	}
	if len(steps) > 0 {
		b.WriteString(" Alur jelajah: " + strings.Join(steps, ", lalu ") + ".")
	}
	return b.String()
}
