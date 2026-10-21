package base

import "time"

// SpecDTO adalah representasi JSON ringkas sebuah spec.
type SpecDTO struct {
	Name           string    `json:"name"`
	Title          string    `json:"title"`
	OpenAPIVersion string    `json:"openapi_version"`
	APIVersion     string    `json:"api_version"`
	Format         string    `json:"format"`
	SourcePath     *string   `json:"source_path"`
	SizeBytes      int       `json:"size_bytes"`
	EndpointCount  int       `json:"endpoint_count"`
	CreatedAt      time.Time `json:"created_at"`
	CreatedByLabel string    `json:"created_by_label"`
	UpdatedAt      time.Time `json:"updated_at"`
	UpdatedByLabel string    `json:"updated_by_label"`
}

type EndpointDTO struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	OperationID *string  `json:"operation_id"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	Deprecated  bool     `json:"deprecated"`
}

type IssueDTO struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Pointer *string `json:"pointer"` // null bila ""
	Line    *int    `json:"line"`    // null bila 0
}

type FileResultDTO struct {
	File           string     `json:"file"`
	SourcePath     *string    `json:"source_path"`
	Name           *string    `json:"name"` // null bila ""
	Status         FileStatus `json:"status"`
	ReasonCode     *string    `json:"reason_code"` // null bila ""
	Reason         *string    `json:"reason"`
	Issues         []IssueDTO `json:"issues"`
	OpenAPIVersion *string    `json:"openapi_version"`
	Title          *string    `json:"title"`
	APIVersion     *string    `json:"api_version"`
	EndpointCount  *int       `json:"endpoint_count"` // null bila skipped/rejected tanpa hasil parse
}

type UploadSummaryDTO struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Rejected  int `json:"rejected"`
}

// ValidateSummaryDTO dipakai validatespec.
type ValidateSummaryDTO struct {
	Valid    int `json:"valid"`
	Skipped  int `json:"skipped"`
	Rejected int `json:"rejected"`
}

// ToSummaryDTO memetakan SpecSummary; waktu UTC.
func ToSummaryDTO(s SpecSummary) SpecDTO {
	return SpecDTO{
		Name: s.Name, Title: s.Title, OpenAPIVersion: s.OpenAPIVersion, APIVersion: s.APIVersion,
		Format: s.Format, SourcePath: s.SourcePath, SizeBytes: s.SizeBytes, EndpointCount: s.EndpointCount,
		CreatedAt: s.CreatedAt.UTC(), CreatedByLabel: s.CreatedByLabel,
		UpdatedAt: s.UpdatedAt.UTC(), UpdatedByLabel: s.UpdatedByLabel,
	}
}

// ToEndpointDTOs tidak pernah mengembalikan nil; Tags tidak pernah nil.
func ToEndpointDTOs(es []Endpoint) []EndpointDTO {
	out := make([]EndpointDTO, len(es))
	for i, e := range es {
		tags := e.Tags
		if tags == nil {
			tags = []string{}
		}
		out[i] = EndpointDTO{Method: e.Method, Path: e.Path, OperationID: e.OperationID,
			Summary: e.Summary, Tags: tags, Deprecated: e.Deprecated}
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ToFileResultDTO memetakan FileResult. Aturan null:
//   - name, reason_code, reason, pointer, line: null bila kosong/0.
//   - status rejected/skipped: openapi_version, title, api_version null bila kosong;
//     endpoint_count null bila 0 (belum ada hasil parse).
//   - status lain (created/updated/unchanged/valid): field-field itu selalu terisi, termasuk 0 endpoint.
func ToFileResultDTO(r FileResult) FileResultDTO {
	issues := make([]IssueDTO, len(r.Issues))
	for i, is := range r.Issues {
		d := IssueDTO{Code: is.Code, Message: is.Message, Pointer: strPtr(is.Pointer)}
		if is.Line != 0 {
			l := is.Line
			d.Line = &l
		}
		issues[i] = d
	}
	out := FileResultDTO{
		File: r.File, SourcePath: r.SourcePath, Name: strPtr(r.Name), Status: r.Status,
		ReasonCode: strPtr(r.ReasonCode), Reason: strPtr(r.Reason), Issues: issues,
	}
	if r.Status == StatusRejected || r.Status == StatusSkipped {
		out.OpenAPIVersion = strPtr(r.OpenAPIVersion)
		out.Title = strPtr(r.Title)
		out.APIVersion = strPtr(r.APIVersion)
		if r.EndpointCount != 0 {
			n := r.EndpointCount
			out.EndpointCount = &n
		}
		return out
	}
	ov, t, av, n := r.OpenAPIVersion, r.Title, r.APIVersion, r.EndpointCount
	out.OpenAPIVersion, out.Title, out.APIVersion, out.EndpointCount = &ov, &t, &av, &n
	return out
}
