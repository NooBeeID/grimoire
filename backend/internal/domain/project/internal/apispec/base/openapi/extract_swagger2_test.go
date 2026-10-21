package openapi

import (
	"strings"
	"testing"
)

func TestExtractEndpoints_Swagger2(t *testing.T) {
	eps := ExtractEndpoints(mustParse(t, "swagger-2.0-petstore.yaml"))
	got := []string{}
	for _, e := range eps {
		got = append(got, e.Method+" "+e.Path)
	}
	if want := "GET /pets|POST /pets|GET /pets/{petId}|DELETE /pets/{petId}"; strings.Join(got, "|") != want {
		t.Fatalf("urutan: %v", got)
	}
	var list *int
	for i, e := range eps {
		if e.Method == "GET" && e.Path == "/pets" {
			list = &i
		}
	}
	if list == nil {
		t.Fatal("GET /pets tidak ada")
	}
	e := eps[*list]
	if e.OperationID == nil || *e.OperationID != "listPets" || e.Summary != "List pets" || strings.Join(e.Tags, ",") != "pets" || e.Deprecated {
		t.Fatalf("%+v", e)
	}
	for _, e := range eps {
		if e.Method == "DELETE" && !e.Deprecated {
			t.Fatalf("deprecated hilang: %+v", e)
		}
	}
	// urutan sama dengan 3.x untuk set path/method setara
	a := ExtractEndpoints(mustParse(t, "swagger-2.0-petstore.yaml"))
	b := ExtractEndpoints(mustParse(t, "swagger-2.0-petstore.json"))
	for i := range a {
		if a[i].Method != b[i].Method || a[i].Path != b[i].Path {
			t.Fatalf("urutan beda: %v vs %v", a, b)
		}
	}
}

func TestExtractEndpoints_Swagger2IgnoresTrace(t *testing.T) {
	for _, e := range ExtractEndpoints(mustParse(t, "swagger-2.0-full.yaml")) {
		if e.Method == "TRACE" {
			t.Fatalf("TRACE ikut diindeks: %+v", e)
		}
	}
}

func TestFindOperation_Swagger2ByOperationID(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-petstore.yaml")
	if m, p, ok := FindOperation(doc, "", "", "createPet"); !ok || m != "POST" || p != "/pets" {
		t.Fatalf("%s %s %v", m, p, ok)
	}
	if _, _, ok := FindOperation(doc, "", "", "nope"); ok {
		t.Fatal("harus tidak ketemu")
	}
}

func TestFindOperation_Swagger2ByMethodPath(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-petstore.yaml")
	if m, p, ok := FindOperation(doc, "delete", "/pets/{petId}", ""); !ok || m != "DELETE" || p != "/pets/{petId}" {
		t.Fatalf("%s %s %v", m, p, ok)
	}
	if _, _, ok := FindOperation(doc, "PUT", "/pets", ""); ok {
		t.Fatal("harus tidak ketemu")
	}
}

func TestFindOperation_Swagger2TraceNotFound(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	if _, _, ok := FindOperation(doc, "TRACE", "/pets/{petId}/photo", ""); ok {
		t.Fatal("TRACE tidak boleh ketemu pada 2.0")
	}
	if _, _, ok := FindOperation(doc, "", "", "traceBroken"); ok {
		t.Fatal("operationId milik trace tidak boleh ketemu pada 2.0")
	}
}
