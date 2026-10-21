package openapi

import (
	"strings"

	"grimoire/internal/domain/project/internal/apispec/base"
)

const summaryMaxRunes = 300

// resolvePathItem mengikuti $ref internal path item (maks 10 lompatan, tahan siklus).
func resolvePathItem(root *Map, item any) (*Map, bool) {
	for hops := 0; hops < 10; hops++ {
		m, ok := asMap(item)
		if !ok {
			return nil, false
		}
		ref, has := m.Get("$ref")
		s, isS := ref.(string)
		if !has || !isS || !strings.HasPrefix(s, "#") {
			return m, true
		}
		target, found := ResolvePointer(root, s)
		if !found {
			return m, true
		}
		// Kunci saudara $ref pada path item tidak digabung; target menjadi path item.
		item = target
	}
	return nil, false
}

// ExtractEndpoints mengekstrak endpoint dari `paths` saja (webhooks tidak diindeks).
// Hasil terurut kanonik (base.SortEndpoints) dan tidak pernah nil.
func ExtractEndpoints(doc Document) []base.Endpoint {
	out := []base.Endpoint{}
	paths, ok := asMap(mustGet(doc.Root, "paths"))
	if !ok {
		return out
	}
	for _, p := range paths.Keys() {
		if strings.HasPrefix(p, "x-") {
			continue
		}
		raw, _ := paths.Get(p)
		item, ok := resolvePathItem(doc.Root, raw)
		if !ok {
			continue
		}
		for _, m := range methodsFor(doc) {
			opv, has := item.Get(m)
			if !has {
				continue
			}
			op, ok := asMap(opv)
			if !ok {
				continue
			}
			out = append(out, endpointOf(strings.ToUpper(m), p, op))
		}
	}
	base.SortEndpoints(out)
	return out
}

func endpointOf(method, path string, op *Map) base.Endpoint {
	e := base.Endpoint{Method: method, Path: path, Tags: []string{}}
	if id, ok := mustGet(op, "operationId").(string); ok && id != "" {
		e.OperationID = &id
	}
	if s, ok := mustGet(op, "summary").(string); ok {
		e.Summary = truncRunesExact(s, summaryMaxRunes)
	}
	if arr, ok := mustGet(op, "tags").([]any); ok {
		seen := map[string]bool{}
		for _, t := range arr {
			if s, ok := t.(string); ok && !seen[s] {
				seen[s] = true
				e.Tags = append(e.Tags, s)
			}
		}
	}
	if d, ok := mustGet(op, "deprecated").(bool); ok {
		e.Deprecated = d
	}
	return e
}

func truncRunesExact(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
