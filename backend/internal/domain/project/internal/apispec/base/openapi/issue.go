package openapi

import (
	"fmt"
	"sort"
	"strings"
)

const (
	CodeInvalidEncoding    = "INVALID_ENCODING"
	CodeEmpty              = "EMPTY"
	CodeSyntax             = "SYNTAX"
	CodeMultiDocument      = "MULTI_DOCUMENT"
	CodeYAMLLimit          = "YAML_LIMIT"
	CodeNotOpenAPI         = "NOT_OPENAPI"
	CodeNotOpenAPI3        = "NOT_OPENAPI_3"
	CodeUnsupportedVersion = "UNSUPPORTED_VERSION"
	CodeInvalidStructure   = "INVALID_STRUCTURE"
	CodeDuplicateOpID      = "DUPLICATE_OPERATION_ID"
	CodeUnresolvedRef      = "UNRESOLVED_REF"
	CodeExternalRef        = "EXTERNAL_REF"
)

// Issue adalah satu masalah pada dokumen.
// Pointer adalah JSON pointer RFC 6901 berawalan "#" ("" bila tidak relevan); Line 1-based, 0 bila tidak diketahui.
// Message sudah memuat lokasi: "<pesan> (baris 12, #/paths/~1pets/get)"; bagian yang kosong dihilangkan.
type Issue struct {
	Code, Message, Pointer string
	Line                   int
}

// compose membangun Issue dengan pesan yang memuat lokasi. Pointer "#" (root) tidak ditulis di pesan.
func compose(code, msg, pointer string, line int) Issue {
	var loc []string
	if line > 0 && pointer != "#" {
		loc = append(loc, fmt.Sprintf("baris %d", line))
	}
	if pointer != "" && pointer != "#" {
		loc = append(loc, pointer)
	}
	full := msg
	if len(loc) > 0 {
		full += " (" + strings.Join(loc, ", ") + ")"
	}
	return Issue{Code: code, Message: full, Pointer: pointer, Line: line}
}

// rawIssue adalah issue sebelum lokasi baris diketahui.
type rawIssue struct{ code, msg, pointer string }

// sortIssues mengurutkan per baris (baris tidak diketahui paling akhir), stabil, lalu memotong ke MaxIssues.
func sortIssues(is []Issue) []Issue {
	sort.SliceStable(is, func(i, j int) bool {
		a, b := is[i].Line, is[j].Line
		if (a == 0) != (b == 0) {
			return b == 0
		}
		return a < b
	})
	if len(is) > MaxIssues {
		is = is[:MaxIssues]
	}
	return is
}
