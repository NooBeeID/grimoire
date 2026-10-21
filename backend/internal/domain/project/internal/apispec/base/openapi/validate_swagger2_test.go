package openapi

import (
	"fmt"
	"strings"
	"testing"
)

// s2 membungkus potongan YAML menjadi dokumen Swagger 2.0 minimal.
func s2(extra string) string {
	return "swagger: \"2.0\"\ninfo: {title: X, version: '1'}\n" + extra
}

func issuesOf(t *testing.T, src string) []Issue {
	t.Helper()
	_, is := Parse([]byte(src), FormatYAML, DefaultLimits)
	return is
}

func wantIssue(t *testing.T, is []Issue, code, ptr string) Issue {
	t.Helper()
	for _, i := range is {
		if i.Code == code && i.Pointer == ptr {
			if i.Line <= 0 {
				t.Errorf("line harus > 0: %+v", i)
			}
			return i
		}
	}
	t.Fatalf("tidak ada issue %s di %s: %+v", code, ptr, is)
	return Issue{}
}

const okOp = "      responses: {'200': {description: ok}}\n"

func TestSwagger2_Petstore_OK(t *testing.T) {
	mustParse(t, "swagger-2.0-petstore.yaml")
	mustParse(t, "swagger-2.0-petstore.json")
}

func TestSwagger2_Full_OK(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	if doc.Title != "Full" || doc.APIVersion != "2.1.0" {
		t.Fatalf("%+v", doc)
	}
}

func TestSwagger2_MissingInfoTitle(t *testing.T) {
	is := issuesOf(t, "swagger: '2.0'\ninfo: {version: '1'}\npaths: {}\n")
	wantIssue(t, is, CodeInvalidStructure, "#/info/title")
	is = issuesOf(t, "swagger: '2.0'\npaths: {}\n")
	wantIssue(t, is, CodeInvalidStructure, "#/info")
}

func TestSwagger2_PathsNotObject(t *testing.T) {
	wantIssue(t, issuesOf(t, s2("paths: [1]\n")), CodeInvalidStructure, "#/paths")
	wantIssue(t, issuesOf(t, s2("")), CodeInvalidStructure, "#/paths")
}

func TestSwagger2_PathWithoutSlash(t *testing.T) {
	wantIssue(t, issuesOf(t, s2("paths:\n  pets:\n    get:\n"+okOp)), CodeInvalidStructure, "#/paths/pets")
	// kunci x- dilewati
	if is := issuesOf(t, s2("paths:\n  x-foo: 1\n")); len(is) != 0 {
		t.Fatalf("%+v", is)
	}
}

func TestSwagger2_BadParams(t *testing.T) {
	_, is := parseFile(t, "swagger-2.0-bad-params.yaml")
	wantIssue(t, is, CodeInvalidStructure, "#/paths/~1noin/get/parameters/0/in")
	wantIssue(t, is, CodeInvalidStructure, "#/paths/~1cookie/get/parameters/0/in")
	wantIssue(t, is, CodeInvalidStructure, "#/paths/~1items~1{id}/get/parameters/0/required")
	wantIssue(t, is, CodeInvalidStructure, "#/paths/~1bodies/post/parameters/1")
	for _, i := range is {
		if i.Code != CodeInvalidStructure {
			t.Errorf("%+v", i)
		}
	}
}

func TestSwagger2_BodyAndFormData(t *testing.T) {
	src := s2("paths:\n  /a:\n    post:\n      parameters:\n        - {name: b, in: body, schema: {type: object}}\n        - {name: f, in: formData, type: string}\n" + okOp)
	wantIssue(t, issuesOf(t, src), CodeInvalidStructure, "#/paths/~1a/post/parameters/1")
	src = s2("paths:\n  /a:\n    post:\n      parameters:\n        - {name: f, in: formData, type: string}\n        - {name: b, in: body, schema: {type: object}}\n" + okOp)
	wantIssue(t, issuesOf(t, src), CodeInvalidStructure, "#/paths/~1a/post/parameters/1")
}

func TestSwagger2_BodyWithoutSchema(t *testing.T) {
	src := s2("paths:\n  /a:\n    post:\n      parameters:\n        - {name: b, in: body}\n" + okOp)
	wantIssue(t, issuesOf(t, src), CodeInvalidStructure, "#/paths/~1a/post/parameters/0/schema")
}

func TestSwagger2_PathLevelBodyPlusOperationBody(t *testing.T) {
	src := s2("paths:\n  /a:\n    parameters:\n      - {name: p, in: body, schema: {type: object}}\n    post:\n      parameters:\n        - {name: q, in: body, schema: {type: object}}\n" + okOp)
	wantIssue(t, issuesOf(t, src), CodeInvalidStructure, "#/paths/~1a/post/parameters/0")
	// nama+in sama -> operasi menimpa, bukan dua body.
	src = s2("paths:\n  /a:\n    parameters:\n      - {name: p, in: body, schema: {type: object}}\n    post:\n      parameters:\n        - {name: p, in: body, schema: {type: object}}\n" + okOp)
	if is := issuesOf(t, src); len(is) != 0 {
		t.Fatalf("%+v", is)
	}
}

func TestSwagger2_DuplicateOperationID(t *testing.T) {
	_, is := parseFile(t, "swagger-2.0-dup-opid.yaml")
	i := wantIssue(t, is, CodeDuplicateOpID, "#/paths/~1b/get/operationId")
	if !strings.Contains(i.Message, "#/paths/~1a/get/operationId") {
		t.Fatalf("%+v", i)
	}
}

func TestSwagger2_MissingResponses(t *testing.T) {
	wantIssue(t, issuesOf(t, s2("paths:\n  /a:\n    get: {}\n")), CodeInvalidStructure, "#/paths/~1a/get/responses")
}

func TestSwagger2_BadResponseCode(t *testing.T) {
	wantIssue(t, issuesOf(t, s2("paths:\n  /a:\n    get:\n      responses: {'20': {description: x}}\n")),
		CodeInvalidStructure, "#/paths/~1a/get/responses/20")
}

func TestSwagger2_RootFields(t *testing.T) {
	wantIssue(t, issuesOf(t, s2("basePath: v1\npaths: {}\n")), CodeInvalidStructure, "#/basePath")
	wantIssue(t, issuesOf(t, s2("schemes: [ftp]\npaths: {}\n")), CodeInvalidStructure, "#/schemes/0")
	wantIssue(t, issuesOf(t, s2("host: 1\npaths: {}\n")), CodeInvalidStructure, "#/host")
	wantIssue(t, issuesOf(t, s2("consumes: [1]\npaths: {}\n")), CodeInvalidStructure, "#/consumes/0")
	wantIssue(t, issuesOf(t, s2("security: {a: []}\npaths: {}\n")), CodeInvalidStructure, "#/security")
	wantIssue(t, issuesOf(t, s2("definitions: []\npaths: {}\n")), CodeInvalidStructure, "#/definitions")
}

func TestSwagger2_TraceIgnored(t *testing.T) {
	src := s2("paths:\n  /a:\n    get:\n" + okOp + "    trace:\n      parameters: nope\n")
	doc, is := Parse([]byte(src), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	if n := len(ExtractEndpoints(doc)); n != 1 {
		t.Fatalf("endpoints=%d", n)
	}
}

func TestSwagger2_MaxRawIssues(t *testing.T) {
	var b strings.Builder
	b.WriteString(s2("paths:\n"))
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "  /p%d:\n    get: {}\n", i)
	}
	is := issuesOf(t, b.String())
	if len(is) != MaxIssues {
		t.Fatalf("len=%d want %d", len(is), MaxIssues)
	}
}
