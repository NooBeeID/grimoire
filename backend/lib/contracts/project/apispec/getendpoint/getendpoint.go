// Package getendpoint adalah kontrak bus project.apispec.get-endpoint (provider T06, consumer T09).
//
// Error Go (membungkus errs.ErrInvalid) hanya untuk request tidak valid: ProjectID bukan uuid;
// tidak ada (Method+Path) maupun OperationID; keduanya diisi; hanya salah satu dari Method/Path;
// Method bukan GET|PUT|POST|DELETE|OPTIONS|HEAD|PATCH|TRACE (provider menormalkan uppercase).
// Clamp: SchemaDepth<=0 -> DefaultSchemaDepth, >MaxSchemaDepth -> MaxSchemaDepth; MaxBytes<=0 -> DefaultMaxBytes.
//
// Hasil "tidak ketemu" (err nil, Found=false):
//
//	Spec "" dan project punya != 1 spec -> Reason SPEC_REQUIRED, Specs = nama spec project.
//	Spec tidak ada                      -> Reason SPEC_NOT_FOUND, Specs.
//	Operasi tidak ada                   -> Reason ENDPOINT_NOT_FOUND, Spec terisi, Similar (maks 5).
//
// Spec "" dan project punya tepat 1 spec -> spec itu dipakai.
package getendpoint

import "encoding/json"

const (
	DefaultSchemaDepth = 4
	MaxSchemaDepth     = 8
	DefaultMaxBytes    = 102400

	ReasonSpecRequired     = "SPEC_REQUIRED"
	ReasonSpecNotFound     = "SPEC_NOT_FOUND"
	ReasonEndpointNotFound = "ENDPOINT_NOT_FOUND"
)

type Request struct {
	ProjectID, Spec, Method, Path, OperationID string
	SchemaDepth                                int
	MaxBytes                                   int
}

type Response struct {
	Found              bool
	Reason             string
	Specs              []string
	Similar            []EndpointRef
	Spec, Method, Path string
	Operation          json.RawMessage
	Servers            json.RawMessage // array
	SecuritySchemes    json.RawMessage // object
	Truncated          bool
	EffectiveDepth     int
	Hint               string // "hasil dipotong; turunkan schema_depth (dipakai N)" bila Truncated
}

type EndpointRef struct{ Method, Path, OperationID, Summary string }
