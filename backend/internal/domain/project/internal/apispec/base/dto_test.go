package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestToFileResultDTO_RejectedBeforeParse(t *testing.T) {
	d := ToFileResultDTO(FileResult{File: "x.txt", Status: StatusSkipped, ReasonCode: ReasonNotSpecFile, Reason: "r"})
	if d.OpenAPIVersion != nil || d.Title != nil || d.APIVersion != nil || d.EndpointCount != nil || d.Name != nil || d.SourcePath != nil {
		t.Fatalf("%+v", d)
	}
	if d.Issues == nil || len(d.Issues) != 0 {
		t.Fatal("issues harus []")
	}
	b, _ := json.Marshal(d)
	if !strings.Contains(string(b), `"issues":[]`) || !strings.Contains(string(b), `"endpoint_count":null`) {
		t.Fatal(string(b))
	}
}

func TestToFileResultDTO_RejectedInvalidSpecIssues(t *testing.T) {
	d := ToFileResultDTO(FileResult{File: "p.json", Name: "p", Status: StatusRejected, ReasonCode: ReasonInvalidSpec, Reason: "bukan OpenAPI",
		Issues: []Issue{{Code: "NOT_OPENAPI", Message: "m", Pointer: "#", Line: 1}, {Code: "YAML_LIMIT", Message: "m2"}}})
	if d.EndpointCount != nil || d.Name == nil || *d.Name != "p" || *d.ReasonCode != ReasonInvalidSpec {
		t.Fatalf("%+v", d)
	}
	if d.Issues[0].Pointer == nil || *d.Issues[0].Line != 1 || d.Issues[1].Pointer != nil || d.Issues[1].Line != nil {
		t.Fatalf("%+v", d.Issues)
	}
}

func TestToFileResultDTO_RejectedLimitKeepsCount(t *testing.T) {
	d := ToFileResultDTO(FileResult{Status: StatusRejected, ReasonCode: ReasonSpecLimitReached, OpenAPIVersion: "3.0.3", Title: "T", APIVersion: "1", EndpointCount: 4})
	if d.EndpointCount == nil || *d.EndpointCount != 4 || *d.Title != "T" {
		t.Fatalf("%+v", d)
	}
}

func TestToFileResultDTO_SuccessKeepsZero(t *testing.T) {
	for _, st := range []FileStatus{StatusCreated, StatusUpdated, StatusUnchanged, StatusValid} {
		d := ToFileResultDTO(FileResult{Status: st, Name: "n", OpenAPIVersion: "3.1.0", Title: "", APIVersion: "1"})
		if d.EndpointCount == nil || *d.EndpointCount != 0 || d.Title == nil || d.OpenAPIVersion == nil || d.ReasonCode != nil || d.Reason != nil {
			t.Fatalf("%s: %+v", st, d)
		}
	}
}

func TestToEndpointDTOs_NeverNil(t *testing.T) {
	if got := ToEndpointDTOs(nil); got == nil || len(got) != 0 {
		t.Fatal("nil")
	}
	got := ToEndpointDTOs([]Endpoint{{Method: "GET", Path: "/a"}})
	if got[0].Tags == nil {
		t.Fatal("tags nil")
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"tags":[]`) {
		t.Fatal(string(b))
	}
}

func TestToSummaryDTO(t *testing.T) {
	loc := time.FixedZone("X", 3600)
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, loc)
	d := ToSummaryDTO(SpecSummary{Name: "n", EndpointCount: 3, CreatedAt: ts, UpdatedAt: ts})
	if d.CreatedAt.Location() != time.UTC || d.EndpointCount != 3 || d.Name != "n" {
		t.Fatalf("%+v", d)
	}
}
