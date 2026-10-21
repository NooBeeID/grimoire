package base

import (
	"strings"
	"testing"
)

func TestSpecMiss(t *testing.T) {
	valid := []string{"billing", "huge", "petstore"}
	tests := []struct {
		name string
		m    SpecMiss
		text string
		code string
	}{
		{"none", SpecMiss{Tool: ToolGetAPIEndpoint}, "Project ini belum punya API spec. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.", "no_specs"},
		{"required", SpecMiss{Required: true, Valid: valid}, "Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs.", "spec_required"},
		{"notfound", SpecMiss{Requested: "petstor", Valid: valid}, "Spec tidak ditemukan: petstor. Spec yang valid: billing, huge, petstore. Nama spec persis seperti di list_api_specs.", "spec_not_found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.m.Result()
			if r.Text != tc.text || !r.IsError || r.TargetType != "api_spec" || r.Target != tc.m.Requested {
				t.Fatalf("%+v", r)
			}
			if r.Summary["error_code"] != tc.code {
				t.Fatalf("summary %v", r.Summary)
			}
			if tc.code != "no_specs" && r.Summary["spec_count"] != 3 {
				t.Fatalf("spec_count %v", r.Summary)
			}
		})
	}
}

func TestSpecMiss_BriefRequested(t *testing.T) {
	m := SpecMiss{Requested: strings.Repeat("x", 100), Valid: []string{"a"}}
	if strings.Contains(m.Text(), strings.Repeat("x", 50)) {
		t.Fatal("harus diringkas")
	}
}
