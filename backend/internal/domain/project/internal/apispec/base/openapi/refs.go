package openapi

import "strings"

// Kunci yang nilainya data literal (bukan skema): $ref di dalamnya tidak diperiksa.
var literalKeys = map[string]bool{"example": true, "default": true, "enum": true, "const": true}

// Kunci yang anak-anaknya adalah NAMA bebas (bukan kata kunci OpenAPI/JSON Schema).
var nameMapKeys = map[string]bool{"properties": true, "patternProperties": true, "$defs": true,
	"definitions": true, "schemas": true, "content": true, "responses": true, "headers": true,
	"callbacks": true, "links": true, "requestBodies": true, "securitySchemes": true, "pathItems": true,
	"encoding": true, "parameters": true, "paths": true, "webhooks": true, "variables": true}

// Tambahan name-map khusus Swagger 2.0 (nameMapKeys tidak berubah agar 3.x identik).
var swagger2NameMapKeys = map[string]bool{"securityDefinitions": true, "scopes": true}

// checkRefs memeriksa semua $ref: internal ("#...") harus terselesaikan; selain itu ditolak (EXTERNAL_REF)
// tanpa pernah di-fetch. Tidak ada resolver, loader, atau HTTP client di package ini.
func checkRefs(doc Document) []rawIssue {
	rc := &refChecker{root: doc.Root, swagger2: doc.IsSwagger2()}
	rc.walk(doc.Root, "#", false)
	return rc.issues
}

type refChecker struct {
	root     *Map
	swagger2 bool // Swagger 2.0: examples adalah peta MIME -> literal; securityDefinitions/scopes name-map
	issues   []rawIssue
}

func (r *refChecker) add(code, msg, ptr string) {
	if len(r.issues) < maxRawIssues {
		r.issues = append(r.issues, rawIssue{code: code, msg: msg, pointer: ptr})
	}
}

// names=true berarti kunci m adalah nama bebas.
func (r *refChecker) walk(x any, ptr string, names bool) {
	switch t := x.(type) {
	case *Map:
		if t == nil {
			return
		}
		if ref, has := t.Get("$ref"); has && !names {
			s, ok := ref.(string)
			switch {
			case !ok:
				r.add(CodeInvalidStructure, "$ref harus berupa string", ptr)
			case !strings.HasPrefix(s, "#"):
				r.add(CodeExternalRef, "referensi eksternal tidak didukung dan tidak di-fetch: "+truncRunes(s, 200), ptr)
			default:
				if _, found := ResolvePointer(r.root, s); !found {
					r.add(CodeUnresolvedRef, "referensi tidak ditemukan: "+truncRunes(s, 200), ptr)
				}
			}
		}
		for _, k := range t.Keys() {
			if !names {
				if strings.HasPrefix(k, "x-") || literalKeys[k] {
					continue
				}
			}
			v, _ := t.Get(k)
			if !names && k == "$ref" {
				continue
			}
			child := joinPtr(ptr, k)
			switch {
			case !names && r.swagger2 && k == "examples":
				// Swagger 2.0: examples = peta MIME -> nilai literal; $ref di dalamnya bukan referensi.
				continue
			case !names && k == "examples":
				r.examples(v, child)
			default:
				r.walk(v, child, !names && (nameMapKeys[k] || (r.swagger2 && swagger2NameMapKeys[k])))
			}
		}
	case []any:
		for i, e := range t {
			r.walk(e, ptr+"/"+itoa(i), false)
		}
	}
}

// examples: array = data literal JSON Schema (dilewati); map = Example Objects (kunci "value" dilewati).
func (r *refChecker) examples(x any, ptr string) {
	m, ok := asMap(x)
	if !ok {
		return
	}
	for _, name := range m.Keys() {
		ex, _ := m.Get(name)
		em, ok := asMap(ex)
		if !ok {
			continue
		}
		ep := joinPtr(ptr, name)
		if ref, has := em.Get("$ref"); has {
			s, isS := ref.(string)
			switch {
			case !isS:
				r.add(CodeInvalidStructure, "$ref harus berupa string", ep)
			case !strings.HasPrefix(s, "#"):
				r.add(CodeExternalRef, "referensi eksternal tidak didukung dan tidak di-fetch: "+truncRunes(s, 200), ep)
			default:
				if _, found := ResolvePointer(r.root, s); !found {
					r.add(CodeUnresolvedRef, "referensi tidak ditemukan: "+truncRunes(s, 200), ep)
				}
			}
		}
	}
}
