package base

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"grimoire/lib/contracts/project/apispec/getendpoint"
)

func foundResp() getendpoint.Response {
	return getendpoint.Response{
		Found: true, Spec: "petstore", Method: "GET", Path: "/pets/{petId}",
		Operation:       json.RawMessage(`{"operationId":"showPetById","a":"<&>","owner":{"$ref":"#/components/schemas/Owner"},"o2":{"$ref":"#/components/schemas/Owner"},"children":{"items":{"$circular":"#/components/schemas/Pet"}}}`),
		Servers:         json.RawMessage(`[{"url":"https://x","$ref":"#/components/schemas/Srv"}]`),
		SecuritySchemes: json.RawMessage(`{"apiKey":{"type":"apiKey","x":{"$ref":5}}}`),
		EffectiveDepth:  4,
	}
}

func lookupDetail(t *testing.T, resp getendpoint.Response, req int) EndpointDetail {
	t.Helper()
	lk, err := NewLookup(EndpointAsk{Spec: "petstore"}, req, 100352, resp)
	if err != nil {
		t.Fatal(err)
	}
	return lk.(EndpointDetail)
}

func TestDetail_JSONCompactOrdered(t *testing.T) {
	d := lookupDetail(t, foundResp(), 4)
	text := d.Text()
	if !json.Valid([]byte(text)) || strings.Contains(text, "\n") {
		t.Fatal(text)
	}
	prev := -1
	for _, k := range []string{`"spec"`, `"method"`, `"path"`, `"schema_depth"`, `"truncated"`, `"markers"`, `"operation"`, `"servers"`, `"securitySchemes"`} {
		i := strings.Index(text, k)
		if i <= prev {
			t.Fatalf("urutan %s: %s", k, text)
		}
		prev = i
	}
	if !strings.Contains(text, `"a":"<&>"`) {
		t.Fatal("html escape")
	}
	if strings.Contains(text, `"hint"`) {
		t.Fatal("hint tanpa truncated")
	}
}

func TestDetail_Markers(t *testing.T) {
	d := lookupDetail(t, foundResp(), 4)
	circ, coll := d.Markers()
	if fmt.Sprint(circ) != "[#/components/schemas/Pet]" || fmt.Sprint(coll) != "[#/components/schemas/Owner #/components/schemas/Srv]" {
		t.Fatalf("%v %v", circ, coll)
	}
	s := d.Summary()
	if s["circular_count"] != 1 || s["collapsed_count"] != 2 {
		t.Fatal(s)
	}
}

func TestDetail_ArraysNeverNull(t *testing.T) {
	resp := foundResp()
	resp.Operation = json.RawMessage(`{"c":{"$circular":"#/x"}}`)
	resp.Servers, resp.SecuritySchemes = nil, nil
	text := lookupDetail(t, resp, 4).Text()
	if !strings.Contains(text, `"collapsed":[]`) || !strings.Contains(text, `"servers":[]`) || !strings.Contains(text, `"securitySchemes":{}`) {
		t.Fatal(text)
	}
}

func TestDetail_NoMarkers(t *testing.T) {
	resp := foundResp()
	resp.Operation = json.RawMessage(`{"operationId":"x"}`)
	resp.Servers, resp.SecuritySchemes = nil, nil
	text := lookupDetail(t, resp, 4).Text()
	if strings.Contains(text, "markers") || strings.Contains(text, "hint") || !strings.Contains(text, `"truncated":false`) {
		t.Fatal(text)
	}
}

func TestDetail_MarkerCaps(t *testing.T) {
	var parts []string
	for i := 0; i < 50; i++ {
		parts = append(parts, fmt.Sprintf(`"c%d":{"$circular":"%s%02d"}`, i, strings.Repeat("r", 98), i))
		parts = append(parts, fmt.Sprintf(`"d%d":{"$ref":"%s%02d"}`, i, strings.Repeat("q", 98), i))
	}
	resp := foundResp()
	resp.Operation = json.RawMessage("{" + strings.Join(parts, ",") + "}")
	d := lookupDetail(t, resp, 4)
	circ, coll := d.Markers()
	if len(circ) > 20 || len(coll) > 20 || markerBytes(circ, coll) > 700 {
		t.Fatalf("%d %d %d", len(circ), len(coll), markerBytes(circ, coll))
	}
	if s := d.Summary(); s["circular_count"] != 50 || s["collapsed_count"] != 51 {
		t.Fatal(s)
	}
}

func TestDetail_Hints(t *testing.T) {
	resp := foundResp()
	resp.Truncated, resp.EffectiveDepth = true, 3
	h := lookupDetail(t, resp, 8).Hint()
	for _, sub := range []string{"schema_depth", "100352", "kedalaman 3", "(diminta 8)", "(mis. 3)"} {
		if !strings.Contains(h, sub) {
			t.Errorf("hint tanpa %q: %s", sub, h)
		}
	}
	resp.EffectiveDepth = 4
	if h := lookupDetail(t, resp, 4).Hint(); !strings.Contains(h, "(mis. 3)") {
		t.Fatal(h)
	}
	resp.EffectiveDepth = 1
	if h := lookupDetail(t, resp, 1).Hint(); !strings.Contains(h, "(mis. 1)") {
		t.Fatal(h)
	}
	resp.Truncated = false
	if lookupDetail(t, resp, 1).Hint() != "" {
		t.Fatal("hint")
	}
}

func TestDetail_Depth0(t *testing.T) {
	resp := foundResp()
	resp.Truncated, resp.EffectiveDepth = true, 0
	resp.Operation = json.RawMessage(`{"$truncated":true,"summary":"x"}`)
	d := lookupDetail(t, resp, 8)
	if !strings.HasPrefix(d.Hint(), "Operasi terlalu besar") {
		t.Fatal(d.Hint())
	}
	text := d.Text()
	if !strings.Contains(text, `"circular":[]`) || !strings.Contains(text, "$truncated: operasi terlalu besar") {
		t.Fatal(text)
	}
	resp.Operation = json.RawMessage(`{"a":1}`)
	h := lookupDetail(t, resp, 8).Hint()
	if strings.Contains(h, "Turunkan schema_depth") || !strings.Contains(h, "kedalaman 0") {
		t.Fatal(h)
	}
}

func TestDetail_ResultAndSummary(t *testing.T) {
	r := lookupDetail(t, foundResp(), 4).Result()
	if r.IsError || r.TargetType != "api_endpoint" || r.Target != "petstore GET /pets/{petId}" || len(r.Summary) != 6 || r.Summary["found"] != true {
		t.Fatalf("%+v", r)
	}
}

func TestNewLookup_InvalidProviderJSON(t *testing.T) {
	for _, op := range []string{"", "null", "{bad"} {
		resp := foundResp()
		resp.Operation = json.RawMessage(op)
		if _, err := NewLookup(EndpointAsk{}, 4, 1, resp); err == nil {
			t.Errorf("operation %q: want error", op)
		}
	}
	resp := foundResp()
	resp.Servers = json.RawMessage("{bad")
	if _, err := NewLookup(EndpointAsk{}, 4, 1, resp); err == nil {
		t.Error("servers")
	}
}

func TestNewLookup_SpecMisses(t *testing.T) {
	lk, err := NewLookup(EndpointAsk{}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonSpecRequired, Specs: []string{"a", "b", "c"}})
	if err != nil {
		t.Fatal(err)
	}
	r := lk.Result()
	if !r.IsError || !strings.Contains(r.Text, "punya 3 API spec") || !strings.Contains(r.Text, "a, b, c") || r.Summary["error_code"] != "spec_required" {
		t.Fatalf("%+v", r)
	}
	lk, _ = NewLookup(EndpointAsk{Spec: "petstor"}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonSpecNotFound, Specs: []string{"petstore"}})
	if !strings.Contains(lk.Result().Text, "Spec tidak ditemukan: petstor") {
		t.Fatal(lk.Result().Text)
	}
	lk, _ = NewLookup(EndpointAsk{Spec: "x"}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonSpecNotFound})
	if r := lk.Result(); !strings.Contains(r.Text, "belum punya API spec") || r.Summary["error_code"] != "no_specs" {
		t.Fatal(r.Text)
	}
}

func TestEndpointMiss_WithSimilar(t *testing.T) {
	lk, err := NewLookup(EndpointAsk{Spec: "petstore", Method: "GET", Path: "/pet/{petId}"}, 4, 1, getendpoint.Response{
		Reason: getendpoint.ReasonEndpointNotFound, Spec: "petstore",
		Similar: []getendpoint.EndpointRef{
			{Method: "GET", Path: "/pets/{petId}", OperationID: "showPetById", Summary: "Info for a specific pet"},
			{Method: "GET", Path: "/pets", OperationID: "listPets", Summary: "List all pets"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}\nSimilar:\n- GET /pets/{petId} — Info for a specific pet [showPetById]\n- GET /pets — List all pets [listPets]\nGunakan salah satu di atas, atau list_api_endpoints(spec=\"petstore\", query=\"pet\") untuk melihat endpoint yang ada."
	r := lk.Result()
	if r.Text != want || !r.IsError || r.Target != "petstore GET /pet/{petId}" || r.Summary["similar_count"] != 2 || r.Summary["error_code"] != "endpoint_not_found" || r.Summary["found"] != false {
		t.Fatalf("%+v", r)
	}
}

func TestEndpointMiss_NoSimilarOperationID(t *testing.T) {
	lk, _ := NewLookup(EndpointAsk{Spec: "petstore", OperationID: "showPetByID"}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonEndpointNotFound})
	want := "Endpoint tidak ditemukan di spec petstore: operation_id showPetByID\nPeriksa ejaan path atau operation_id, atau gunakan list_api_endpoints(spec=\"petstore\", query=\"showPetByID\") untuk melihat endpoint yang ada."
	r := lk.Result()
	if r.Text != want || r.Target != "petstore operation_id=showPetByID" || r.Summary["similar_count"] != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestEndpointMiss_SuggestQuery(t *testing.T) {
	tests := map[string]string{"/pets/{petId}": "pets", "/{id}": "", "/": "", "/v1/pets": "v1", "/" + strings.Repeat("a", 60): strings.Repeat("a", 37) + "..."}
	for path, want := range tests {
		if got := (EndpointMiss{Ask: EndpointAsk{Path: path}}).SuggestQuery(); got != want {
			t.Errorf("%q: got %q want %q", path, got, want)
		}
	}
	if strings.Contains((EndpointMiss{Spec: "s", Ask: EndpointAsk{Path: "/{id}"}}).Text(), "query=") {
		t.Fatal("query harus dihilangkan")
	}
}

func TestEndpointMiss_CapAndSanitize(t *testing.T) {
	var sim []getendpoint.EndpointRef
	for i := 0; i < 7; i++ {
		sim = append(sim, getendpoint.EndpointRef{Method: "GET", Path: "/p", Summary: "a\nb\t" + strings.Repeat("z", 300)})
	}
	lk, _ := NewLookup(EndpointAsk{Method: "GET", Path: "/q"}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonEndpointNotFound, Similar: sim})
	m := lk.(EndpointMiss)
	if m.Spec != "" || len(m.Similar) != 5 || strings.Count(m.Text(), "\n- ") != 5 || strings.Contains(m.Text(), "\t") {
		t.Fatal(m.Text())
	}
	lk, _ = NewLookup(EndpointAsk{Spec: "ask"}, 4, 1, getendpoint.Response{Reason: getendpoint.ReasonEndpointNotFound})
	if lk.(EndpointMiss).Spec != "ask" {
		t.Fatal("fallback spec")
	}
}

func TestNewLookup_UnknownReason(t *testing.T) {
	for _, r := range []string{"WEIRD", ""} {
		if _, err := NewLookup(EndpointAsk{}, 4, 1, getendpoint.Response{Reason: r}); err == nil {
			t.Errorf("reason %q: want error", r)
		}
	}
}
