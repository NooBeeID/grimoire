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
//
// Swagger 2.0: Servers dibentuk dari schemes x host + basePath (schemes kosong -> https; host kosong ->
// hanya basePath; basePath kosong -> "/"). SecuritySchemes berasal dari securityDefinitions yang disebut
// security efektif. Operation adalah operasi 2.0 ter-inline apa adanya (tidak dinormalisasi ke 3.x).
// OpenAPIVersion ("2.0"), Consumes dan Produces (array JSON efektif) hanya terisi untuk Swagger 2.0 dan
// kosong ("" / nil) untuk OpenAPI 3.x; konsumen yang mengabaikannya tetap benar (perubahan aditif).
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
	// Penanda versi. "2.0" hanya untuk Swagger 2.0; "" untuk OpenAPI 3.x.
	OpenAPIVersion string
	// Array JSON consumes/produces efektif (operasi menimpa root; tidak ada keduanya -> []).
	// Hanya untuk Swagger 2.0; nil untuk OpenAPI 3.x.
	Consumes json.RawMessage
	Produces json.RawMessage
}

type EndpointRef struct{ Method, Path, OperationID, Summary string }
