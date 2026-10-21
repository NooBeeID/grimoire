package openapi

import "testing"

func TestRefs_Swagger2Unresolved(t *testing.T) {
	_, is := parseFile(t, "swagger-2.0-unresolved-ref.yaml")
	wantIssue(t, is, CodeUnresolvedRef, "#/paths/~1pets/get/responses/200/schema")
	// di dalam definitions bersarang
	src := s2("paths: {}\ndefinitions:\n  A:\n    type: object\n    properties:\n      b:\n        $ref: '#/definitions/Nope'\n")
	wantIssue(t, issuesOf(t, src), CodeUnresolvedRef, "#/definitions/A/properties/b")
}

func TestRefs_Swagger2ExamplesAreLiteral(t *testing.T) {
	mustParse(t, "swagger-2.0-examples-ref-key.yaml")
}

func TestRefs_Swagger2SecurityDefinitionsNameMap(t *testing.T) {
	src := s2(`paths: {}
securityDefinitions:
  $ref:
    type: oauth2
    flow: implicit
    authorizationUrl: https://a.example.com
    scopes:
      $ref: nama scope
      other: x
`)
	if is := issuesOf(t, src); len(is) != 0 {
		t.Fatalf("%+v", is)
	}
}

func TestRefs_Swagger2CircularOK(t *testing.T) {
	mustParse(t, "swagger-2.0-circular.yaml")
}

func TestRefs_Swagger2ParametersAndResponses(t *testing.T) {
	src := s2("paths:\n  /a:\n    get:\n      parameters:\n        - $ref: '#/parameters/Nope'\n      responses:\n        '404':\n          $ref: '#/responses/Nope'\n")
	is := issuesOf(t, src)
	wantIssue(t, is, CodeUnresolvedRef, "#/paths/~1a/get/parameters/0")
	wantIssue(t, is, CodeUnresolvedRef, "#/paths/~1a/get/responses/404")
}

// Kontrol 3.x: examples bentuk Example Object tetap diperiksa seperti sebelumnya.
func TestRefs_3xExamplesUnchanged(t *testing.T) {
	src := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    get:\n      responses:\n        '200':\n          description: x\n          content:\n            application/json:\n              examples:\n                e: {$ref: '#/nope'}\n"
	wantIssue(t, issuesOf(t, src), CodeUnresolvedRef, "#/paths/~1a/get/responses/200/content/application~1json/examples/e")
}
