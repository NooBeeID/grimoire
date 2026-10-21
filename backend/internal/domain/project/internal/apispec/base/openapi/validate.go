package openapi

import (
	"encoding/json"
	"regexp"
	"strings"
)

const maxRawIssues = 200

var (
	responseCodeRe = regexp.MustCompile(`^([1-5][0-9]{2}|[1-5]XX|default)$`)
	paramIns       = map[string]bool{"query": true, "header": true, "path": true, "cookie": true}
	opMethods      = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}
	componentKeys  = []string{"schemas", "responses", "parameters", "examples", "requestBodies", "headers",
		"securitySchemes", "links", "callbacks", "pathItems"}
)

type validator struct {
	doc    Document
	is30   bool
	issues []rawIssue
	opIDs  map[string]string // operationId -> pointer pertama
}

func (v *validator) add(code, msg, ptr string) {
	if len(v.issues) < maxRawIssues {
		v.issues = append(v.issues, rawIssue{code: code, msg: msg, pointer: ptr})
	}
}

func (v *validator) bad(msg, ptr string) { v.add(CodeInvalidStructure, msg, ptr) }

func isStr(x any) bool  { _, ok := x.(string); return ok }
func isBool(x any) bool { _, ok := x.(bool); return ok }

func isNumber(x any) bool {
	switch x.(type) {
	case json.Number, float64:
		return true
	}
	return false
}

// validate memeriksa struktur OpenAPI 3.0/3.1 secara toleran (field tak dikenal dan x-* diabaikan).
// Tipe skema (nullable vs type array) sengaja tidak divalidasi atau dinormalkan.
func validate(doc Document) []rawIssue {
	v := &validator{doc: doc, is30: strings.HasPrefix(doc.OpenAPIVersion, "3.0"), opIDs: map[string]string{}}
	root := doc.Root

	if info, ok := asMap(mustGet(root, "info")); !ok {
		v.bad("info wajib ada dan berupa object", "#/info")
	} else {
		if !isStr(mustGet(info, "title")) {
			v.bad("info.title wajib ada dan berupa string", "#/info/title")
		}
		if !isStr(mustGet(info, "version")) {
			v.bad("info.version wajib ada dan berupa string", "#/info/version")
		}
	}

	paths, hasPaths := root.Get("paths")
	_, hasWebhooks := root.Get("webhooks")
	_, hasComponents := root.Get("components")
	if v.is30 && !hasPaths {
		v.bad("paths wajib ada pada OpenAPI 3.0", "#/paths")
	}
	if !v.is30 && !hasPaths && !hasWebhooks && !hasComponents {
		v.bad("OpenAPI 3.1 butuh minimal salah satu dari paths, webhooks, components", "#")
	}

	if hasPaths {
		pm, ok := asMap(paths)
		if !ok {
			v.bad("paths harus berupa object", "#/paths")
		} else {
			for _, k := range pm.Keys() {
				if strings.HasPrefix(k, "x-") {
					continue
				}
				ptr := joinPtr("#/paths", k)
				if !strings.HasPrefix(k, "/") {
					v.bad("kunci path harus diawali '/': "+truncRunes(k, 80), ptr)
					continue
				}
				item, _ := pm.Get(k)
				v.pathItem(item, ptr)
			}
		}
	}
	if hasWebhooks {
		wm, ok := asMap(mustGet(root, "webhooks"))
		if !ok {
			v.bad("webhooks harus berupa object", "#/webhooks")
		} else {
			for _, k := range wm.Keys() {
				if strings.HasPrefix(k, "x-") {
					continue
				}
				item, _ := wm.Get(k)
				v.pathItem(item, joinPtr("#/webhooks", k))
			}
		}
	}

	if hasComponents {
		cm, ok := asMap(mustGet(root, "components"))
		if !ok {
			v.bad("components harus berupa object", "#/components")
		} else {
			for _, k := range componentKeys {
				if sub, has := cm.Get(k); has {
					if _, ok := asMap(sub); !ok {
						v.bad("components."+k+" harus berupa object", joinPtr("#/components", k))
					}
				}
			}
		}
	}

	if sv, has := root.Get("servers"); has {
		v.objectArray(sv, "#/servers", "servers", "url")
	}
	if tg, has := root.Get("tags"); has {
		v.objectArray(tg, "#/tags", "tags", "name")
	}
	if sec, has := root.Get("security"); has {
		v.securityArray(sec, "#/security")
	}
	return v.issues
}

func (v *validator) objectArray(x any, ptr, label, requiredStr string) {
	arr, ok := x.([]any)
	if !ok {
		v.bad(label+" harus berupa array", ptr)
		return
	}
	for i, e := range arr {
		ep := ptr + "/" + itoa(i)
		em, ok := asMap(e)
		if !ok {
			v.bad("elemen "+label+" harus berupa object", ep)
			continue
		}
		if requiredStr != "" && !isStr(mustGet(em, requiredStr)) {
			v.bad(label+"."+requiredStr+" wajib ada dan berupa string", ep+"/"+requiredStr)
		}
	}
}

func (v *validator) securityArray(x any, ptr string) {
	arr, ok := x.([]any)
	if !ok {
		v.bad("security harus berupa array", ptr)
		return
	}
	for i, e := range arr {
		if _, ok := asMap(e); !ok {
			v.bad("elemen security harus berupa object", ptr+"/"+itoa(i))
		}
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func (v *validator) pathItem(item any, ptr string) {
	pm, ok := asMap(item)
	if !ok {
		v.bad("path item harus berupa object", ptr)
		return
	}
	if r, has := pm.Get("$ref"); has && !isStr(r) {
		v.bad("$ref harus berupa string", ptr+"/$ref")
	}
	if params, has := pm.Get("parameters"); has {
		v.parameters(params, ptr+"/parameters")
	}
	if sv, has := pm.Get("servers"); has {
		v.objectArray(sv, ptr+"/servers", "servers", "url")
	}
	for _, m := range opMethods {
		op, has := pm.Get(m)
		if !has {
			continue
		}
		v.operation(op, ptr+"/"+m)
	}
}

func (v *validator) parameters(x any, ptr string) {
	arr, ok := x.([]any)
	if !ok {
		v.bad("parameters harus berupa array", ptr)
		return
	}
	for i, p := range arr {
		pp := ptr + "/" + itoa(i)
		pm, ok := asMap(p)
		if !ok {
			v.bad("parameter harus berupa object", pp)
			continue
		}
		if r, has := pm.Get("$ref"); has {
			if !isStr(r) {
				v.bad("$ref harus berupa string", pp+"/$ref")
			}
			continue
		}
		if !isStr(mustGet(pm, "name")) {
			v.bad("parameter.name wajib ada dan berupa string", pp+"/name")
		}
		in, _ := mustGet(pm, "in").(string)
		if !paramIns[in] {
			v.bad("parameter.in harus salah satu dari query|header|path|cookie", pp+"/in")
			continue
		}
		if in == "path" {
			if req, ok := mustGet(pm, "required").(bool); !ok || !req {
				v.bad("parameter path harus required: true", pp+"/required")
			}
		}
	}
}

func (v *validator) operation(x any, ptr string) {
	op, ok := asMap(x)
	if !ok {
		v.bad("operasi harus berupa object", ptr)
		return
	}
	if id, has := op.Get("operationId"); has {
		s, isS := id.(string)
		switch {
		case !isS:
			v.bad("operationId harus berupa string", ptr+"/operationId")
		case s != "":
			if first, dup := v.opIDs[s]; dup {
				v.add(CodeDuplicateOpID, "operationId duplikat "+quoteShort(s)+" (pertama di "+first+")", ptr+"/operationId")
			} else {
				v.opIDs[s] = ptr + "/operationId"
			}
		}
	}
	if tg, has := op.Get("tags"); has {
		arr, ok := tg.([]any)
		if !ok {
			v.bad("tags harus berupa array string", ptr+"/tags")
		} else {
			for i, t := range arr {
				if !isStr(t) {
					v.bad("tags harus berupa array string", ptr+"/tags/"+itoa(i))
				}
			}
		}
	}
	if d, has := op.Get("deprecated"); has && !isBool(d) {
		v.bad("deprecated harus berupa boolean", ptr+"/deprecated")
	}
	for _, k := range []string{"summary", "description"} {
		if s, has := op.Get(k); has && !isStr(s) {
			v.bad(k+" harus berupa string", ptr+"/"+k)
		}
	}
	if p, has := op.Get("parameters"); has {
		v.parameters(p, ptr+"/parameters")
	}
	if rb, has := op.Get("requestBody"); has {
		if _, ok := asMap(rb); !ok {
			v.bad("requestBody harus berupa object", ptr+"/requestBody")
		}
	}
	if sv, has := op.Get("servers"); has {
		v.objectArray(sv, ptr+"/servers", "servers", "url")
	}
	if sec, has := op.Get("security"); has {
		v.securityArray(sec, ptr+"/security")
	}
	rs, hasR := op.Get("responses")
	if !hasR {
		if v.is30 {
			v.bad("responses wajib ada pada OpenAPI 3.0", ptr+"/responses")
		}
		return
	}
	rm, ok := asMap(rs)
	if !ok {
		v.bad("responses harus berupa object", ptr+"/responses")
		return
	}
	for _, code := range rm.Keys() {
		if strings.HasPrefix(code, "x-") {
			continue
		}
		cp := joinPtr(ptr+"/responses", code)
		if !responseCodeRe.MatchString(code) {
			v.bad("kode response tidak valid: "+quoteShort(code), cp)
			continue
		}
		if r, _ := rm.Get(code); r != nil {
			if _, ok := asMap(r); !ok {
				v.bad("response harus berupa object", cp)
			}
		} else {
			v.bad("response harus berupa object", cp)
		}
	}
}

func quoteShort(s string) string {
	b, _ := json.Marshal(truncRunes(s, 80))
	return string(b)
}
