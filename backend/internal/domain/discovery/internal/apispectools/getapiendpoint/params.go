package getapiendpoint

import (
	"encoding/json"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type rawArgs struct {
	Spec        *string `json:"spec"`
	Method      *string `json:"method"`
	Path        *string `json:"path"`
	OperationID *string `json:"operation_id"`
	SchemaDepth *int    `json:"schema_depth"`
}

// Params adalah argumen tervalidasi.
type Params struct {
	Ask         base.EndpointAsk
	SchemaDepth int
}

var allowedArgs = []string{"spec", "method", "path", "operation_id", "schema_depth"}

const schemaJSON = `{
  "type": "object",
  "properties": {
    "spec": { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; boleh kosong bila project hanya punya satu spec" },
    "method": { "type": "string", "maxLength": 7, "description": "GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, atau TRACE (huruf besar/kecil bebas); wajib bersama path" },
    "path": { "type": "string", "maxLength": 2048, "description": "path persis seperti di spec, mis. /pets/{petId}; wajib bersama method" },
    "operation_id": { "type": "string", "maxLength": 200, "description": "operationId; alternatif dari method + path" },
    "schema_depth": { "type": "integer", "minimum": 1, "maximum": 8, "default": 4, "description": "kedalaman pembukaan $ref; turunkan bila hasil terpotong" }
  },
  "additionalProperties": false
}`

func schema() json.RawMessage { return json.RawMessage(schemaJSON) }

// Parse memvalidasi argumen; error pertama menang. Superset aturan provider T06.
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var r rawArgs
	if e := base.DecodeStrict(raw, &r, allowedArgs); e != nil {
		return Params{}, e
	}
	spec, e := base.StrArg("spec", r.Spec, 80, "Nama spec dari list_api_specs; boleh dikosongkan bila project hanya punya satu spec.")
	if e != nil {
		return Params{}, e
	}
	method, e := base.StrArg("method", r.Method, 32, "Huruf besar/kecil bebas.")
	if e != nil {
		return Params{}, e
	}
	if method != "" {
		m, ok := base.NormalizeMethod(method)
		if !ok {
			return Params{}, &base.ArgError{Field: "method", Reason: "harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE",
				Got: base.Brief(method), Hint: "Huruf besar/kecil bebas."}
		}
		method = m
	}
	path, e := base.StrArg("path", r.Path, 2048, "Tulis path persis seperti di list_api_endpoints, mis. /pets/{petId}.")
	if e != nil {
		return Params{}, e
	}
	if path != "" && path[0] != '/' {
		return Params{}, &base.ArgError{Field: "path", Reason: "harus diawali /", Got: base.Brief(path),
			Hint: "Tulis path persis seperti di list_api_endpoints, mis. /pets/{petId}."}
	}
	opID, e := base.StrArg("operation_id", r.OperationID, 200, "operationId persis seperti di list_api_endpoints.")
	if e != nil {
		return Params{}, e
	}
	depth, e := base.IntArg("schema_depth", r.SchemaDepth, getendpoint.DefaultSchemaDepth, 1, getendpoint.MaxSchemaDepth,
		"Turunkan nilai bila hasil terpotong; maksimum 8.")
	if e != nil {
		return Params{}, e
	}

	hasMP := method != "" || path != ""
	hasOp := opID != ""
	switch {
	case hasMP && hasOp:
		return Params{}, &base.ArgError{Field: "operation_id", Reason: "tidak boleh diisi bersama method/path (pilih salah satu: method + path atau operation_id)",
			Got: base.Brief(opID), Hint: "Hapus operation_id atau hapus method dan path."}
	case !hasMP && !hasOp:
		return Params{}, &base.ArgError{Field: "method", Reason: "wajib diisi bersama path, atau isi operation_id",
			Hint: "Cari endpoint dengan list_api_endpoints."}
	case method != "" && path == "":
		return Params{}, &base.ArgError{Field: "path", Reason: "wajib diisi bersama method", Hint: "Cari path dengan list_api_endpoints."}
	case path != "" && method == "":
		return Params{}, &base.ArgError{Field: "method", Reason: "wajib diisi bersama path", Hint: "Cari method dengan list_api_endpoints."}
	}
	return Params{
		Ask:         base.EndpointAsk{Spec: spec, Method: method, Path: path, OperationID: opID},
		SchemaDepth: depth,
	}, nil
}
