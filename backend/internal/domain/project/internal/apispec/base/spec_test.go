package base

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 9, 0, 0, 0, time.FixedZone("X", 7*3600))

func TestNewSpec_HashAndSize(t *testing.T) {
	a := Actor{ID: "u1", Label: "Budi"}
	sp := NewSpec("p1", "pay", nil, DocMeta{Title: "T", APIVersion: "1", OpenAPIVersion: "3.0.3"}, "openapi: 3.0.3", FormatYAML, a, t0)
	if sp.Hash != HashContent("openapi: 3.0.3") || sp.SizeBytes != len("openapi: 3.0.3") {
		t.Fatalf("%+v", sp)
	}
	if sp.CreatedAt.Location() != time.UTC || sp.UpdatedAt != sp.CreatedAt || sp.CreatedBy != "u1" || sp.UpdatedByLabel != "Budi" {
		t.Fatalf("%+v", sp)
	}
}

func TestSpec_ApplyContentKeepsCreated(t *testing.T) {
	a := Actor{ID: "u1", Label: "Budi"}
	b := Actor{ID: "u2", Label: "Sari"}
	sp := NewSpec("p1", "pay", nil, DocMeta{}, "a", FormatYAML, a, t0)
	later := t0.Add(time.Hour)
	p := "x/y.json"
	sp.ApplyContent(&p, DocMeta{Title: "N"}, "bb", FormatJSON, b, later)
	if sp.CreatedBy != "u1" || sp.UpdatedBy != "u2" || sp.UpdatedByLabel != "Sari" || sp.Title != "N" ||
		sp.SizeBytes != 2 || sp.Format != FormatJSON || sp.Hash != HashContent("bb") || !sp.UpdatedAt.Equal(later) || sp.SourcePath != &p {
		t.Fatalf("%+v", sp)
	}
	if !sp.CreatedAt.Equal(t0) {
		t.Fatal("created_at berubah")
	}
}

func TestSpec_Rename(t *testing.T) {
	a := Actor{ID: "u1", Label: "Budi"}
	sp := NewSpec("p1", "pay", nil, DocMeta{}, "a", FormatYAML, a, t0)
	before := sp
	if sp.Rename("pay", Actor{ID: "u2"}, t0.Add(time.Hour)) || sp != before {
		t.Fatal("rename ke nama sama harus no-op")
	}
	if !sp.Rename("pay2", Actor{ID: "u2", Label: "S"}, t0.Add(time.Hour)) || sp.Name != "pay2" || sp.UpdatedBy != "u2" {
		t.Fatalf("%+v", sp)
	}
}

func TestMediaTypeAndFileName(t *testing.T) {
	cases := []struct {
		s         Spec
		mt, fname string
	}{
		{Spec{Name: "payment", Format: FormatYAML}, "application/yaml; charset=utf-8", "payment.yaml"},
		{Spec{Name: "v1.2_x-y", Format: FormatJSON}, "application/json; charset=utf-8", "v1.2_x-y.json"},
		{Spec{Name: "x", Format: "toml"}, "application/octet-stream", "x.txt"},
	}
	for _, c := range cases {
		if c.s.MediaType() != c.mt || c.s.FileName() != c.fname {
			t.Errorf("%+v: %q %q", c.s, c.s.MediaType(), c.s.FileName())
		}
	}
}

func TestMethodsAndSort(t *testing.T) {
	if !ValidMethod("GET") || ValidMethod("get") || ValidMethod("CONNECT") {
		t.Fatal("ValidMethod")
	}
	if MethodOrder("GET") != 0 || MethodOrder("TRACE") != 7 || MethodOrder("X") != len(Methods) {
		t.Fatal("MethodOrder")
	}
	es := []Endpoint{{Method: "DELETE", Path: "/b"}, {Method: "POST", Path: "/a"}, {Method: "GET", Path: "/b"}, {Method: "GET", Path: "/a"}, {Method: "PATCH", Path: "/a"}}
	SortEndpoints(es)
	want := []string{"GET /a", "POST /a", "PATCH /a", "GET /b", "DELETE /b"}
	for i, e := range es {
		if e.Method+" "+e.Path != want[i] {
			t.Fatalf("%d: %v", i, es)
		}
	}
}
