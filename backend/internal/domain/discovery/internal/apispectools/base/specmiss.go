package base

import (
	"fmt"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// NoSpecsText adalah pesan project tanpa spec untuk hasil sukses (bukan error).
const NoSpecsText = "Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files."

// SpecMiss adalah hasil "spec tidak ada / spec wajib" (K14).
type SpecMiss struct {
	Tool      string   // tool pemanggil
	Requested string   // nama spec dari agent; "" bila tidak diisi
	Required  bool     // true = spec tidak diisi dan project punya != 1 spec
	Valid     []string // nama spec valid dari provider (sudah urut)
}

func (m SpecMiss) code() string {
	switch {
	case len(m.Valid) == 0:
		return "no_specs"
	case m.Required:
		return "spec_required"
	}
	return "spec_not_found"
}

// Text menyusun pesan sesuai cabang no_specs, spec_required, spec_not_found.
func (m SpecMiss) Text() string {
	switch m.code() {
	case "no_specs":
		return "Project ini belum punya API spec. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files."
	case "spec_required":
		return fmt.Sprintf("Project ini punya %d API spec; isi argumen spec. Spec yang valid: %s. Lihat detailnya dengan list_api_specs.", len(m.Valid), JoinNames(m.Valid, 20))
	}
	return fmt.Sprintf("Spec tidak ditemukan: %s. Spec yang valid: %s. Nama spec persis seperti di list_api_specs.", Brief(m.Requested), JoinNames(m.Valid, 20))
}

// Result adalah hasil isError dengan metadata saja.
func (m SpecMiss) Result() toolkit.Result {
	sum := map[string]any{"error_code": m.code()}
	if m.code() != "no_specs" {
		sum["spec_count"] = len(m.Valid)
	}
	return toolkit.Result{Text: m.Text(), IsError: true, TargetType: TargetTypeSpec, Target: m.Requested, Summary: sum}
}
