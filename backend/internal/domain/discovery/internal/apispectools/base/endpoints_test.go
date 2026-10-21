package base

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/contracts/project/apispec/listendpoints"
)

func ep(spec, method, path, opID, summary string, tags []string, dep bool) listendpoints.Endpoint {
	return listendpoints.Endpoint{Spec: spec, Method: method, Path: path, OperationID: opID, Summary: summary, Tags: tags, Deprecated: dep}
}

func page(f Filter, offset, total int, items []listendpoints.Endpoint, budget int) EndpointPage {
	p, miss := NewEndpointPage(f, offset, listendpoints.Response{SpecFound: true, Items: items, Total: total}, budget)
	if miss != nil {
		panic("unexpected miss")
	}
	return p
}

const hintLine = "Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id)."

func TestEndpointPage_GroupedBySpec(t *testing.T) {
	items := []listendpoints.Endpoint{
		ep("billing", "GET", "/invoices", "listInvoices", "List invoices", []string{"invoices"}, false),
		ep("billing", "POST", "/invoices/{id}/pay", "", "Pay an invoice", []string{"invoices"}, false),
		ep("petstore", "POST", "/pets", "createPet", "Create a pet", []string{"pets", "admin"}, true),
	}
	p := page(Filter{}, 0, 3, items, 100000)
	want := "Endpoint 1-3 dari 3 | spec: semua\n== billing ==\nGET /invoices — List invoices [listInvoices] (invoices)\nPOST /invoices/{id}/pay — Pay an invoice (invoices)\n== petstore ==\nPOST /pets — Create a pet [createPet] (pets, admin) [deprecated]\n\n" + hintLine
	if p.Text() != want {
		t.Fatalf("got:\n%s", p.Text())
	}
	r := p.Result()
	if r.IsError || r.TargetType != "api_spec" || r.Target != "" {
		t.Fatalf("%+v", r)
	}
	wantSum := map[string]any{"count": 3, "total": 3, "next_offset": -1, "spec_groups": 2, "truncated": false}
	if fmt.Sprint(r.Summary) != fmt.Sprint(wantSum) {
		t.Fatalf("%v", r.Summary)
	}
}

func TestEndpointPage_HeaderFilters(t *testing.T) {
	p := page(Filter{Spec: "petstore", Tag: "pets", Query: "pet"}, 2, 4, []listendpoints.Endpoint{ep("petstore", "GET", "/pets", "", "", nil, false)}, 0)
	text := p.Text()
	if !strings.HasPrefix(text, "Endpoint 3-3 dari 4 | spec: petstore | tag: pets | query: \"pet\"\n== petstore ==\nGET /pets\n") {
		t.Fatal(text)
	}
	if p.Result().Target != "petstore" {
		t.Fatal("target")
	}
}

func TestEndpointPage_RowVariants(t *testing.T) {
	long := strings.Repeat("é", 300)
	tests := []struct {
		name string
		e    listendpoints.Endpoint
		want string
	}{
		{"no tag no op", ep("s", "GET", "/health", "", "Health check", nil, false), "GET /health — Health check"},
		{"no summary", ep("s", "GET", "/health", "healthCheck", " \n ", nil, false), "GET /health [healthCheck]"},
		{"tags sanitized", ep("s", "GET", "/x", "", "", []string{"a\nb", "", "c"}, false), "GET /x (a b, c)"},
		{"summary flat", ep("s", "GET", "/x", "", "a\nb\tc", nil, false), "GET /x — a b c"},
	}
	for _, tc := range tests {
		if got := formatRow(tc.e); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	row := formatRow(ep("s", "GET", strings.Repeat("p", 1000), "", long, nil, false))
	if strings.Count(row, "p") != 297 || !strings.Contains(row, strings.Repeat("é", 157)+"...") || !utf8.ValidString(row) {
		t.Fatal("clip")
	}
	tags := make([]string, 200)
	for i := range tags {
		tags[i] = "tag"
	}
	if r := formatRow(ep("s", "GET", "/x", "", "", tags, false)); utf8.RuneCountInString(r) > len("GET /x ()")+120 {
		t.Fatal("tags clip")
	}
}

func TestEndpointPage_Marker(t *testing.T) {
	items := []listendpoints.Endpoint{ep("a", "GET", "/1", "", "", nil, false), ep("a", "GET", "/2", "", "", nil, false)}
	p := page(Filter{}, 0, 6, items, 100000)
	if p.NextOffset() != 2 {
		t.Fatal(p.NextOffset())
	}
	want := "\n\n[ada 4 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]\n" + hintLine
	if !strings.HasSuffix(p.Text(), want) {
		t.Fatal(p.Text())
	}
	if s := p.Result().Summary; s["truncated"] != true || s["next_offset"] != 2 {
		t.Fatal(s)
	}
}

func TestEndpointPage_NextOffsetIgnoresProvider(t *testing.T) {
	p, _ := NewEndpointPage(Filter{}, 0, listendpoints.Response{SpecFound: true, Total: 2, Truncated: true, NextOffset: 999,
		Items: []listendpoints.Endpoint{ep("a", "GET", "/1", "", "", nil, false), ep("a", "GET", "/2", "", "", nil, false)}}, 0)
	if p.NextOffset() != -1 {
		t.Fatal(p.NextOffset())
	}
}

func manyItems(n int) []listendpoints.Endpoint {
	var items []listendpoints.Endpoint
	for i := 0; i < n; i++ {
		items = append(items, ep("a", "GET", fmt.Sprintf("/path/%03d", i), "", "Ringkasan endpoint nomor tertentu", nil, false))
	}
	return items
}

func TestEndpointPage_BudgetTruncates(t *testing.T) {
	p := page(Filter{}, 0, 50, manyItems(50), 1000)
	r := p.Result()
	shown := r.Summary["count"].(int)
	if shown <= 0 || shown >= 50 || len(r.Text) > 1000 {
		t.Fatalf("shown=%d len=%d", shown, len(r.Text))
	}
	if p.NextOffset() != shown || !strings.Contains(r.Text, fmt.Sprintf("[ada %d endpoint lagi", 50-shown)) || !strings.Contains(r.Text, fmt.Sprintf("offset=%d", shown)) {
		t.Fatal(r.Text)
	}
	if !strings.HasPrefix(r.Text, fmt.Sprintf("Endpoint 1-%d dari 50", shown)) {
		t.Fatal(r.Text)
	}
}

func TestEndpointPage_BudgetAtLeastOneRow(t *testing.T) {
	p := page(Filter{}, 0, 5, manyItems(5), 10)
	if _, shown := p.Lines(); shown != 1 || p.NextOffset() != 1 {
		t.Fatal(shown)
	}
}

func TestEndpointPage_BudgetReserveSufficient(t *testing.T) {
	for budget := 700; budget <= 3000; budget += 37 {
		p := page(Filter{Spec: "a"}, 1000000, 1000299, manyItems(300), budget)
		if _, shown := p.Lines(); shown >= 2 && len(p.Text()) > budget {
			t.Fatalf("budget=%d len=%d", budget, len(p.Text()))
		}
	}
}

func TestEndpointPage_GroupHeaderWithRow(t *testing.T) {
	items := []listendpoints.Endpoint{ep("a", "GET", "/1", "", "", nil, false), ep("b", "GET", "/"+strings.Repeat("x", 250), "", "", nil, false)}
	p := page(Filter{}, 0, 2, items, 330)
	if strings.Contains(p.Text(), "== b ==") {
		t.Fatal(p.Text())
	}
}

func TestEndpointPage_Total0(t *testing.T) {
	p := page(Filter{}, 0, 0, nil, 0)
	if r := p.Result(); r.IsError || r.Text != NoSpecsText || r.Summary["total"] != 0 || r.Summary["next_offset"] != -1 {
		t.Fatalf("%+v", r)
	}
	p = page(Filter{Spec: "petstore", Tag: "x", Query: "y"}, 0, 0, nil, 0)
	if !strings.HasPrefix(p.Text(), `Tidak ada endpoint yang cocok (spec: petstore, tag: x, query: "y"). Hapus filter tag/query,`) {
		t.Fatal(p.Text())
	}
	p = page(Filter{Spec: "petstore"}, 5, 0, nil, 0)
	if p.Result().IsError || !strings.HasPrefix(p.Text(), "Tidak ada endpoint yang cocok (spec: petstore).") {
		t.Fatal(p.Text())
	}
}

func TestEndpointPage_OffsetOutOfRange(t *testing.T) {
	for _, off := range []int{6, 5000} {
		r := page(Filter{}, off, 6, nil, 0).Result()
		want := fmt.Sprintf("Argumen tidak valid: offset %d melewati total 6 endpoint. Gunakan offset 0 sampai 5.", off)
		if !r.IsError || r.Text != want || r.Summary["error_code"] != "offset_out_of_range" || r.Summary["total"] != 6 || len(r.Summary) != 2 {
			t.Fatalf("%+v", r)
		}
	}
}

func TestEndpointPage_SpecMiss(t *testing.T) {
	_, miss := NewEndpointPage(Filter{Spec: "petstor"}, 0, listendpoints.Response{Specs: []string{"billing", "petstore"}}, 0)
	if miss == nil || miss.Tool != ToolListAPIEndpoints || miss.Requested != "petstor" || miss.Required {
		t.Fatalf("%+v", miss)
	}
	r := miss.Result()
	if !r.IsError || !strings.Contains(r.Text, "Spec tidak ditemukan: petstor") || !strings.Contains(r.Text, "billing, petstore") {
		t.Fatal(r.Text)
	}
	_, miss = NewEndpointPage(Filter{Spec: "x"}, 0, listendpoints.Response{}, 0)
	if !strings.Contains(miss.Text(), "belum punya API spec") {
		t.Fatal(miss.Text())
	}
}

func TestEndpointPage_OrderAndUnicode(t *testing.T) {
	items := []listendpoints.Endpoint{ep("z", "GET", "/b", "", "", nil, false), ep("a", "GET", "/a", "", strings.Repeat("🙂", 300), nil, false)}
	text := page(Filter{}, 0, 2, items, 0).Text()
	if strings.Index(text, "== z ==") > strings.Index(text, "== a ==") || !utf8.ValidString(text) {
		t.Fatal(text)
	}
}
