// Package base memuat entity dan rule murni module api-spec-tools (tanpa I/O).
package base

import "strings"

const (
	ToolListAPISpecs     = "list_api_specs"
	ToolListAPIEndpoints = "list_api_endpoints"
	ToolGetAPIEndpoint   = "get_api_endpoint"
	TargetTypeSpec       = "api_spec"
	TargetTypeEndpoint   = "api_endpoint"
)

// ProjectArgAliases adalah nama argumen project yang ditolak dengan pesan khusus (K5).
var ProjectArgAliases = []string{"project_id", "projectId", "project"}

// Methods urutan kanonik, sama dengan T06.
var Methods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}

// NormalizeMethod: TrimSpace + ToUpper; ok=false bila bukan anggota Methods.
func NormalizeMethod(s string) (string, bool) {
	m := strings.ToUpper(strings.TrimSpace(s))
	for _, k := range Methods {
		if k == m {
			return m, true
		}
	}
	return "", false
}
