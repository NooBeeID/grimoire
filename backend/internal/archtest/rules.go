// Package archtest menegakkan batas import backend (rules.txt bagian 4)
// sebagai fungsi murni di atas daftar edge import.
//
// Tabel aturan (urutan evaluasi per edge, berhenti di pelanggaran pertama):
//
//	4.1  domain -> domain lain
//	4.2  module -> module lain (domain sama)
//	4.3  feature -> feature lain dalam module yang sama
//	4.6  infra -> domain
//	4.5  lib/contracts -> domain, infra, internal lain, cmd, lib/constants, lib/util
//	4.9  lib/** (selain contracts) -> internal/**, cmd/**
//
// Rule 4.4 tidak punya cek tersendiri: consumer yang mengimpor provider selalu
// melintasi module atau domain, sehingga tertangkap 4.1/4.2.
//
// Sengaja tidak dicakup (tidak bisa dideteksi dari graf import saja, tetap lewat
// review): 4.7 (cmd tanpa logika bisnis), 4.8 (arah handler -> usecase ->
// repository), 5.x (layering di dalam feature), dan impor base/ ke feature.
//
// Tidak ada allowlist: pelanggaran diperbaiki, bukan dikecualikan.
package archtest

import (
	"fmt"
	"sort"
	"strings"
)

// ModulePath harus sama dengan baris `module` di backend/go.mod (dijaga TestModulePathMatchesGoMod).
const ModulePath = "grimoire"

// ID aturan = nomor di rules.txt. Dijaga TestRuleCatalog_ExistsInRulesTxt.
const (
	Rule41 = "4.1" // domain -> domain lain
	Rule42 = "4.2" // module -> module lain
	Rule43 = "4.3" // feature -> feature lain dalam module
	Rule45 = "4.5" // lib/contracts -> internal/**, cmd/**, lib/constants, lib/util
	Rule46 = "4.6" // infra -> domain
	Rule49 = "4.9" // lib/** (selain contracts) -> internal/**, cmd/**
)

// Import adalah satu edge graf import; kedua sisi berupa import path penuh.
type Import struct{ From, To string }

// Violation adalah satu edge yang melanggar satu aturan.
type Violation struct {
	Rule, From, To, Reason string
}

// String: "rules.txt 4.2: <From> imports <To> (<Reason>)".
func (v Violation) String() string {
	return fmt.Sprintf("rules.txt %s: %s imports %s (%s)", v.Rule, v.From, v.To, v.Reason)
}

type kind int

const (
	kExternal kind = iota
	kDomain
	kInfra
	kInternalOther
	kContracts
	kLib
	kCmd
)

type loc struct {
	kind                    kind
	domain, module, feature string
	lib                     string
}

func classify(path string) loc {
	if path != ModulePath && !strings.HasPrefix(path, ModulePath+"/") {
		return loc{kind: kExternal}
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(path, ModulePath), "/")
	if rel == "" {
		return loc{kind: kExternal}
	}
	p := strings.Split(rel, "/")
	switch p[0] {
	case "cmd":
		return loc{kind: kCmd}
	case "lib":
		if len(p) < 2 {
			return loc{kind: kExternal}
		}
		if p[1] == "contracts" {
			return loc{kind: kContracts}
		}
		return loc{kind: kLib, lib: p[1]}
	case "internal":
		if len(p) < 2 {
			return loc{kind: kInternalOther}
		}
		switch p[1] {
		case "infra":
			return loc{kind: kInfra}
		case "domain":
			l := loc{kind: kDomain}
			if len(p) < 3 {
				return l
			}
			l.domain = p[2]
			if len(p) >= 5 && p[3] == "internal" {
				l.module = p[4]
				if len(p) >= 6 && p[5] != "base" {
					l.feature = p[5]
				}
			}
			return l
		}
		return loc{kind: kInternalOther}
	}
	return loc{kind: kExternal}
}

func violationFor(from, to string) (Violation, bool) {
	f, t := classify(from), classify(to)
	mk := func(rule, reason string) (Violation, bool) {
		return Violation{Rule: rule, From: from, To: to, Reason: reason}, true
	}
	if f.kind == kDomain && t.kind == kDomain {
		if f.domain != t.domain {
			return mk(Rule41, "domain -> domain lain; berinteraksi lewat infra/bus + lib/contracts, lihat rules.txt 4.4")
		}
		if f.module != "" && t.module != "" {
			if f.module != t.module {
				return mk(Rule42, "module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4")
			}
			if f.feature != "" && t.feature != "" && f.feature != t.feature {
				return mk(Rule43, "feature -> feature lain dalam module; bagi lewat base/")
			}
		}
	}
	if f.kind == kInfra && t.kind == kDomain {
		return mk(Rule46, "infra -> domain; infra tidak tahu bisnis")
	}
	if f.kind == kContracts {
		switch t.kind {
		case kDomain, kInfra, kInternalOther, kCmd:
			return mk(Rule45, "lib/contracts hanya boleh mengimpor stdlib, lib/common, lib/contracts")
		case kLib:
			if t.lib == "constants" || t.lib == "util" {
				return mk(Rule45, "lib/contracts hanya boleh mengimpor stdlib, lib/common, lib/contracts")
			}
		}
	}
	if f.kind == kLib {
		switch t.kind {
		case kDomain, kInfra, kInternalOther, kCmd:
			return mk(Rule49, "lib -> internal/cmd; lib harus netral")
		}
	}
	return Violation{}, false
}

// Check murni. Edge ke luar ModulePath (stdlib, pihak ketiga) dan edge From == To diabaikan.
// Hasil bebas duplikat dan terurut (Rule, From, To).
func Check(imports []Import) []Violation {
	seen := map[Violation]struct{}{}
	var out []Violation
	for _, im := range imports {
		if im.From == im.To {
			continue
		}
		v, ok := violationFor(im.From, im.To)
		if !ok {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return out
}
