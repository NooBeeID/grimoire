package openapi

import "strings"

// swagger2Methods adalah method path item Swagger 2.0 yang diindeks (tanpa trace, D5).
var swagger2Methods = []string{"get", "put", "post", "delete", "options", "head", "patch"}

var (
	swagger2ParamIns = map[string]bool{"query": true, "header": true, "path": true, "formData": true, "body": true}
	swagger2Schemes  = map[string]bool{"http": true, "https": true, "ws": true, "wss": true}
)

// swagger2Param adalah parameter efektif (setelah $ref internal diselesaikan) beserta pointer sumbernya.
type swagger2Param struct {
	key, in, ptr string
}

// validateSwagger2 memeriksa struktur Swagger 2.0 secara toleran (field tak dikenal dan x-* diabaikan).
// allOf, discriminator, collectionFormat, x-nullable tidak divalidasi semantik (D7).
func validateSwagger2(doc Document) []rawIssue {
	v := &validator{doc: doc, opIDs: map[string]string{}}
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

	if h, has := root.Get("host"); has && !isStr(h) {
		v.bad("host harus berupa string", "#/host")
	}
	if b, has := root.Get("basePath"); has {
		if s, ok := b.(string); !ok || !strings.HasPrefix(s, "/") {
			v.bad("basePath harus berupa string yang diawali '/'", "#/basePath")
		}
	}
	if sc, has := root.Get("schemes"); has {
		v.swagger2Schemes(sc, "#/schemes")
	}
	for _, k := range []string{"consumes", "produces"} {
		if x, has := root.Get(k); has {
			v.stringArray(x, "#/"+k, k)
		}
	}
	for _, k := range []string{"definitions", "parameters", "responses", "securityDefinitions"} {
		if x, has := root.Get(k); has {
			if _, ok := asMap(x); !ok {
				v.bad(k+" harus berupa object", "#/"+k)
			}
		}
	}
	if sec, has := root.Get("security"); has {
		v.securityArray(sec, "#/security")
	}

	paths, hasPaths := root.Get("paths")
	if !hasPaths {
		v.bad("paths wajib ada pada Swagger 2.0", "#/paths")
		return v.issues
	}
	pm, ok := asMap(paths)
	if !ok {
		v.bad("paths harus berupa object", "#/paths")
		return v.issues
	}
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
		v.swagger2PathItem(item, ptr)
	}
	return v.issues
}

func (v *validator) stringArray(x any, ptr, label string) {
	arr, ok := x.([]any)
	if !ok {
		v.bad(label+" harus berupa array string", ptr)
		return
	}
	for i, e := range arr {
		if !isStr(e) {
			v.bad(label+" harus berupa array string", ptr+"/"+itoa(i))
		}
	}
}

func (v *validator) swagger2Schemes(x any, ptr string) {
	arr, ok := x.([]any)
	if !ok {
		v.bad("schemes harus berupa array", ptr)
		return
	}
	for i, e := range arr {
		s, isS := e.(string)
		if !isS || !swagger2Schemes[s] {
			v.bad("schemes harus berisi http|https|ws|wss", ptr+"/"+itoa(i))
		}
	}
}

func (v *validator) swagger2PathItem(item any, ptr string) {
	pm, ok := asMap(item)
	if !ok {
		v.bad("path item harus berupa object", ptr)
		return
	}
	if r, has := pm.Get("$ref"); has && !isStr(r) {
		v.bad("$ref harus berupa string", ptr+"/$ref")
	}
	var pathParams []swagger2Param
	if params, has := pm.Get("parameters"); has {
		pathParams = v.swagger2Parameters(params, ptr+"/parameters")
	}
	for _, m := range swagger2Methods {
		op, has := pm.Get(m)
		if !has {
			continue
		}
		v.swagger2Operation(op, ptr+"/"+m, pathParams)
	}
}

// swagger2Parameters memeriksa array parameter dan mengembalikan parameter efektif (untuk pemeriksaan body).
func (v *validator) swagger2Parameters(x any, ptr string) []swagger2Param {
	arr, ok := x.([]any)
	if !ok {
		v.bad("parameters harus berupa array", ptr)
		return nil
	}
	var out []swagger2Param
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
				continue
			}
			// parameter terpisah lewat $ref: diperiksa di tempat definisinya; di sini hanya dipakai untuk body.
			if t, found := ResolvePointer(v.doc.Root, r.(string)); found {
				if tm, ok := asMap(t); ok {
					in, _ := mustGet(tm, "in").(string)
					name, _ := mustGet(tm, "name").(string)
					out = append(out, swagger2Param{key: in + "\x00" + name, in: in, ptr: pp})
				}
			}
			continue
		}
		name, _ := mustGet(pm, "name").(string)
		if !isStr(mustGet(pm, "name")) {
			v.bad("parameter.name wajib ada dan berupa string", pp+"/name")
		}
		in, _ := mustGet(pm, "in").(string)
		if !swagger2ParamIns[in] {
			v.bad("parameter.in harus salah satu dari query|header|path|formData|body", pp+"/in")
			continue
		}
		switch in {
		case "path":
			if req, ok := mustGet(pm, "required").(bool); !ok || !req {
				v.bad("parameter path harus required: true", pp+"/required")
			}
		case "body":
			if _, ok := asMap(mustGet(pm, "schema")); !ok {
				v.bad("parameter body wajib punya schema berupa object", pp+"/schema")
			}
		}
		out = append(out, swagger2Param{key: in + "\x00" + name, in: in, ptr: pp})
	}
	return out
}

func (v *validator) swagger2Operation(x any, ptr string, pathParams []swagger2Param) {
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
		v.stringArray(tg, ptr+"/tags", "tags")
	}
	if d, has := op.Get("deprecated"); has && !isBool(d) {
		v.bad("deprecated harus berupa boolean", ptr+"/deprecated")
	}
	for _, k := range []string{"summary", "description"} {
		if s, has := op.Get(k); has && !isStr(s) {
			v.bad(k+" harus berupa string", ptr+"/"+k)
		}
	}
	for _, k := range []string{"consumes", "produces"} {
		if x, has := op.Get(k); has {
			v.stringArray(x, ptr+"/"+k, k)
		}
	}
	if sc, has := op.Get("schemes"); has {
		v.swagger2Schemes(sc, ptr+"/schemes")
	}
	var opParams []swagger2Param
	if p, has := op.Get("parameters"); has {
		opParams = v.swagger2Parameters(p, ptr+"/parameters")
	}
	v.bodyRules(pathParams, opParams)
	if sec, has := op.Get("security"); has {
		v.securityArray(sec, ptr+"/security")
	}

	rs, hasR := op.Get("responses")
	if !hasR {
		v.bad("responses wajib ada pada Swagger 2.0", ptr+"/responses")
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

// bodyRules: maksimal satu body per operasi (path-level digabung operasi, operasi menimpa name+in sama)
// dan body tidak boleh bersamaan dengan formData.
func (v *validator) bodyRules(pathParams, opParams []swagger2Param) {
	have := map[string]bool{}
	for _, p := range opParams {
		have[p.key] = true
	}
	merged := make([]swagger2Param, 0, len(pathParams)+len(opParams))
	for _, p := range pathParams {
		if !have[p.key] {
			merged = append(merged, p)
		}
	}
	merged = append(merged, opParams...)
	var bodyPtr, formPtr string
	for _, p := range merged {
		switch p.in {
		case "body":
			if bodyPtr != "" {
				v.bad("hanya satu parameter body yang diperbolehkan per operasi (pertama di "+bodyPtr+")", p.ptr)
				continue
			}
			bodyPtr = p.ptr
			if formPtr != "" {
				v.bad("parameter body tidak boleh bersamaan dengan formData", p.ptr)
			}
		case "formData":
			if formPtr == "" {
				formPtr = p.ptr
				if bodyPtr != "" {
					v.bad("parameter formData tidak boleh bersamaan dengan body", p.ptr)
				}
			}
		}
	}
}
