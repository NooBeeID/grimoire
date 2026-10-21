package openapi

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrOperationNotFound dikembalikan ResolveOperation bila (method, path) tidak ada.
var ErrOperationNotFound = errors.New("openapi: operasi tidak ditemukan")

// Resolved adalah hasil resolusi satu operasi untuk agent.
type Resolved struct {
	Operation       json.RawMessage
	Servers         json.RawMessage // selalu array JSON
	SecuritySchemes json.RawMessage // selalu object JSON
	Truncated       bool
	EffectiveDepth  int
}

const maxInlineExpansions = 20000

// FindOperation mencari operasi lewat (method, path) persis (path termasuk "{param}") atau operationID.
// operationID dipakai bila terisi. Method dinormalkan ke huruf besar. ok=false bila tidak ada.
func FindOperation(doc Document, method, path, operationID string) (method2, path2 string, ok bool) {
	paths, isMap := asMap(mustGet(doc.Root, "paths"))
	if !isMap {
		return "", "", false
	}
	if operationID != "" {
		for _, p := range paths.Keys() {
			if strings.HasPrefix(p, "x-") {
				continue
			}
			raw, _ := paths.Get(p)
			item, ok := resolvePathItem(doc.Root, raw)
			if !ok {
				continue
			}
			for _, m := range opMethods {
				if op, ok := asMap(mustGet(item, m)); ok {
					if id, _ := op.Get("operationId"); id == operationID {
						return strings.ToUpper(m), p, true
					}
				}
			}
		}
		return "", "", false
	}
	raw, has := paths.Get(path)
	if !has || strings.HasPrefix(path, "x-") {
		return "", "", false
	}
	item, okItem := resolvePathItem(doc.Root, raw)
	if !okItem {
		return "", "", false
	}
	lm := strings.ToLower(method)
	for _, m := range opMethods {
		if m == lm {
			if _, ok := asMap(mustGet(item, m)); ok {
				return strings.ToUpper(m), path, true
			}
		}
	}
	return "", "", false
}

type inliner struct {
	root     *Map
	maxDepth int
	budget   int
	cut      bool // true bila ada $ref yang tidak di-inline karena batas kedalaman
}

func hasString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// inline membuka $ref internal secara rekursif. level adalah kedalaman ref saat ini; setiap masuk ke
// objek tujuan $ref menambah 1. Melebihi maxDepth -> {"$ref"}; ref yang sedang di stack -> {"$circular"}.
func (in *inliner) inline(x any, level int, stack []string) any {
	switch t := x.(type) {
	case *Map:
		if t == nil {
			return nil
		}
		if ref, ok := mustGet(t, "$ref").(string); ok && strings.HasPrefix(ref, "#") {
			target, found := ResolvePointer(in.root, ref)
			if found {
				if hasString(stack, ref) {
					m := NewMap()
					m.Set("$circular", ref)
					return m
				}
				if level+1 > in.maxDepth {
					in.cut = true
				}
				if level+1 > in.maxDepth || in.budget <= 0 {
					m := NewMap()
					m.Set("$ref", ref)
					return m
				}
				in.budget--
				inl := in.inline(target, level+1, append(stack[:len(stack):len(stack)], ref))
				if im, ok := inl.(*Map); ok && t.Len() > 1 {
					res := im.clone()
					for _, k := range t.Keys() {
						if k == "$ref" {
							continue
						}
						v, _ := t.Get(k)
						res.Set(k, in.inline(v, level, stack))
					}
					return res
				}
				return inl
			}
		}
		out := NewMap()
		for _, k := range t.Keys() {
			v, _ := t.Get(k)
			out.Set(k, in.inline(v, level, stack))
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = in.inline(e, level, stack)
		}
		return out
	default:
		return x
	}
}

// paramKey mengidentifikasi parameter (name+in), menyelesaikan $ref internal lebih dulu.
func paramKey(root *Map, p any) string {
	m, ok := asMap(p)
	if !ok {
		return ""
	}
	if ref, ok := mustGet(m, "$ref").(string); ok {
		if t, found := ResolvePointer(root, ref); found {
			if tm, ok := asMap(t); ok {
				m = tm
			}
		}
	}
	name, _ := m.Get("name")
	in, _ := m.Get("in")
	ns, _ := name.(string)
	is, _ := in.(string)
	if ns == "" && is == "" {
		return ""
	}
	return is + "\x00" + ns
}

// mergedOperation menggabungkan parameter level path ke operasi; operasi menang untuk name+in yang sama.
func mergedOperation(root, item, op *Map) *Map {
	pathParams, _ := mustGet(item, "parameters").([]any)
	if len(pathParams) == 0 {
		return op
	}
	opParams, _ := mustGet(op, "parameters").([]any)
	have := map[string]bool{}
	for _, p := range opParams {
		if k := paramKey(root, p); k != "" {
			have[k] = true
		}
	}
	merged := make([]any, 0, len(pathParams)+len(opParams))
	for _, p := range pathParams {
		if k := paramKey(root, p); k != "" && have[k] {
			continue
		}
		merged = append(merged, p)
	}
	merged = append(merged, opParams...)
	out := op.clone()
	out.Set("parameters", merged)
	return out
}

func firstServers(op, item, root *Map) []any {
	for _, m := range []*Map{op, item, root} {
		if s, ok := mustGet(m, "servers").([]any); ok {
			return s
		}
	}
	return []any{}
}

func securityNames(op, root *Map) []string {
	var sec any
	if v, has := op.Get("security"); has {
		sec = v
	} else {
		sec, _ = root.Get("security")
	}
	arr, _ := sec.([]any)
	var names []string
	for _, e := range arr {
		if m, ok := asMap(e); ok {
			for _, k := range m.Keys() {
				if !hasString(names, k) {
					names = append(names, k)
				}
			}
		}
	}
	return names
}

// ResolveOperation menghasilkan operasi dengan $ref internal di-inline sampai kedalaman depth.
// Hasil dipotong bertahap (depth turun) bila melebihi maxBytes; maxBytes <= 0 berarti tanpa batas.
func ResolveOperation(doc Document, method, path string, depth, maxBytes int) (Resolved, error) {
	root := doc.Root
	paths, ok := asMap(mustGet(root, "paths"))
	if !ok {
		return Resolved{}, ErrOperationNotFound
	}
	raw, has := paths.Get(path)
	if !has {
		return Resolved{}, ErrOperationNotFound
	}
	item, ok := resolvePathItem(root, raw)
	if !ok {
		return Resolved{}, ErrOperationNotFound
	}
	op, ok := asMap(mustGet(item, strings.ToLower(method)))
	if !ok || !hasString(opMethods, strings.ToLower(method)) {
		return Resolved{}, ErrOperationNotFound
	}
	merged := mergedOperation(root, item, op)
	servers := firstServers(op, item, root)
	names := securityNames(op, root)
	schemes, _ := asMap(mustGet(mustMap(mustGet(root, "components")), "securitySchemes"))

	if depth < 0 {
		depth = 0
	}
	build := func(d int) (Resolved, bool, error) {
		in := &inliner{root: root, maxDepth: d, budget: maxInlineExpansions}
		opJSON, err := json.Marshal(in.inline(merged, 0, nil))
		if err != nil {
			return Resolved{}, false, err
		}
		svJSON, err := json.Marshal(in.inline(servers, 0, nil))
		if err != nil {
			return Resolved{}, false, err
		}
		sm := NewMap()
		for _, n := range names {
			if v, has := schemes.Get(n); has {
				sm.Set(n, in.inline(v, 0, nil))
			}
		}
		scJSON, err := json.Marshal(sm)
		if err != nil {
			return Resolved{}, false, err
		}
		return Resolved{Operation: opJSON, Servers: svJSON, SecuritySchemes: scJSON, EffectiveDepth: d}, in.cut, nil
	}

	size := func(r Resolved) int { return len(r.Operation) + len(r.Servers) + len(r.SecuritySchemes) }
	// Telusuri dari kedalaman terkecil naik: ukuran tumbuh monoton dan biaya build tumbuh
	// eksponensial, jadi berhenti pada level pertama yang melebihi maxBytes menghindari
	// membangun pohon terdalam yang pasti terlalu besar.
	var best, first Resolved
	haveBest := false
	for d := 0; d <= depth; d++ {
		r, cut, err := build(d)
		if err != nil {
			return Resolved{}, err
		}
		if d == 0 {
			first = r
		}
		if maxBytes > 0 && size(r) > maxBytes {
			break
		}
		best, haveBest = r, true
		if !cut {
			// Tidak ada $ref yang tertahan kedalaman: level lebih dalam menghasilkan hasil sama.
			best.EffectiveDepth = depth
			return best, nil
		}
	}
	if haveBest {
		best.Truncated = best.EffectiveDepth < depth
		return best, nil
	}
	{
		r := first
		// depth 0 pun terlalu besar: ringkasan saja.
		t := NewMap()
		t.Set("$truncated", true)
		if s, ok := op.Get("summary"); ok {
			t.Set("summary", s)
		}
		if id, ok := op.Get("operationId"); ok {
			t.Set("operationId", id)
		}
		tj, err := json.Marshal(t)
		if err != nil {
			return Resolved{}, err
		}
		r.Operation = tj
		r.Truncated, r.EffectiveDepth = true, 0
		return r, nil
	}
}

func mustMap(v any) *Map {
	m, _ := asMap(v)
	return m
}
