package base

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/project/apispec/listspecs"
)

var ts = time.Date(2026, 10, 20, 8, 15, 0, 0, time.UTC)

func mkItems(n int) listspecs.Response {
	var r listspecs.Response
	for i := 1; i <= n; i++ {
		r.Items = append(r.Items, listspecs.Spec{Name: fmt.Sprintf("s%02d", i), Title: fmt.Sprintf("Service %02d", i), ApiVersion: "1.0.0", OpenapiVersion: "3.0.3", EndpointCount: 4, UpdatedAt: ts})
	}
	return r
}

func TestCatalog_Empty(t *testing.T) {
	for _, resp := range []listspecs.Response{{}, {Items: []listspecs.Spec{}}} {
		c := NewSpecCatalog(resp, 1000)
		r := c.Result()
		if r.Text != NoSpecsText || r.IsError || r.TargetType != "api_spec" || r.Target != "" {
			t.Fatalf("%+v", r)
		}
		if r.Summary["count"] != 0 || r.Summary["endpoint_count"] != 0 || r.Summary["shown"] != 0 {
			t.Fatalf("%v", r.Summary)
		}
	}
}

func TestCatalog_Golden(t *testing.T) {
	resp := listspecs.Response{Items: []listspecs.Spec{
		{Name: "billing", Title: "Billing API", ApiVersion: "2.1.0", OpenapiVersion: "3.0.3", EndpointCount: 2, UpdatedAt: ts},
		{Name: "huge", Title: "Huge Report", ApiVersion: "1.0.0", OpenapiVersion: "3.0.3", EndpointCount: 1, UpdatedAt: ts.Add(time.Minute)},
		{Name: "petstore", Title: "Swagger Petstore", ApiVersion: "1.0.0", OpenapiVersion: "3.0.3", EndpointCount: 3, UpdatedAt: ts.Add(2 * time.Minute)},
	}}
	c := NewSpecCatalog(resp, TextBudget(102400))
	want := "3 API spec di project ini:\n" +
		"- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-20 08:15 UTC\n" +
		"- huge: Huge Report | versi 1.0.0 | OpenAPI 3.0.3 | 1 endpoint | diperbarui 2026-10-20 08:16 UTC\n" +
		"- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint | diperbarui 2026-10-20 08:17 UTC\n" +
		"Langkah berikut: list_api_endpoints(spec=\"<name>\") untuk daftar endpoint, lalu get_api_endpoint untuk detail."
	if c.Text() != want {
		t.Fatalf("got:\n%s", c.Text())
	}
	s := c.Result().Summary
	if s["count"] != 3 || s["endpoint_count"] != 6 || s["shown"] != 3 {
		t.Fatalf("%v", s)
	}
}

func TestCatalog_EmptyFields(t *testing.T) {
	c := NewSpecCatalog(listspecs.Response{Items: []listspecs.Spec{{Name: "legacy", EndpointCount: 0}}}, 0)
	if !strings.Contains(c.Text(), "1 API spec di project ini:\n- legacy: - | versi - | OpenAPI - | 0 endpoint | diperbarui -") {
		t.Fatal(c.Text())
	}
}

func TestCatalog_Sanitize(t *testing.T) {
	jkt := time.FixedZone("J", 7*3600)
	r := listspecs.Response{Items: []listspecs.Spec{{Name: "a", Title: "A\r\nB\tC   D", ApiVersion: strings.Repeat("v", 100), UpdatedAt: time.Date(2026, 10, 20, 15, 15, 0, 0, jkt)}}}
	text := NewSpecCatalog(r, 0).Text()
	if !strings.Contains(text, "- a: A B C D | versi "+strings.Repeat("v", 37)+"... |") || !strings.Contains(text, "08:15 UTC") {
		t.Fatal(text)
	}
	if n := strings.Count(text, "\n"); n != 2 {
		t.Fatalf("newline=%d", n)
	}
}

func TestCatalog_BudgetTruncates(t *testing.T) {
	resp := mkItems(20)
	full := NewSpecCatalog(resp, 0).Text()
	budget := len(full) / 2
	c := NewSpecCatalog(resp, budget)
	r := c.Result()
	if len(r.Text) > budget {
		t.Fatalf("len %d > %d", len(r.Text), budget)
	}
	shown := r.Summary["shown"].(int)
	if shown <= 0 || shown >= 20 {
		t.Fatalf("shown=%d", shown)
	}
	for i := 1; i <= shown; i++ {
		if !strings.Contains(r.Text, fmt.Sprintf("- s%02d:", i)) {
			t.Fatalf("missing s%02d", i)
		}
	}
	if strings.Contains(r.Text, fmt.Sprintf("- s%02d:", shown+1)) {
		t.Fatal("prefix harus berurutan")
	}
	if !strings.Contains(r.Text, fmt.Sprintf("[%d spec lain tidak ditampilkan;", 20-shown)) {
		t.Fatal(r.Text)
	}
	if !strings.HasSuffix(r.Text, "untuk detail.") || r.Summary["endpoint_count"] != 80 || r.Summary["count"] != 20 {
		t.Fatalf("%v", r.Summary)
	}
}

func TestCatalog_BudgetExactFit(t *testing.T) {
	resp := mkItems(5)
	full := NewSpecCatalog(resp, 0).Text()
	if c := NewSpecCatalog(resp, len(full)); c.Result().Summary["shown"] != 5 || strings.Contains(c.Text(), "tidak ditampilkan") {
		t.Fatal("exact fit")
	}
	c := NewSpecCatalog(resp, len(full)-1)
	if c.Result().Summary["shown"].(int) >= 5 || len(c.Text()) > len(full)-1 {
		t.Fatal("minus one")
	}
}

func TestCatalog_BudgetTiny(t *testing.T) {
	c := NewSpecCatalog(mkItems(3), 200)
	r := c.Result()
	if r.IsError || r.Summary["shown"] != 0 || !strings.Contains(r.Text, "[3 spec lain tidak ditampilkan;") || !strings.HasPrefix(r.Text, "3 API spec") {
		t.Fatalf("%+v", r)
	}
}

func TestCatalog_BudgetCountsBytes(t *testing.T) {
	var resp listspecs.Response
	for i := 0; i < 10; i++ {
		resp.Items = append(resp.Items, listspecs.Spec{Name: fmt.Sprintf("s%d", i), Title: strings.Repeat("日", 40), UpdatedAt: ts})
	}
	if text := NewSpecCatalog(resp, 700).Text(); len(text) > 700 {
		t.Fatalf("len=%d", len(text))
	}
}

func TestCatalog_DefaultBudget(t *testing.T) {
	if text := NewSpecCatalog(mkItems(2000), TextBudget(102400)).Text(); len(text) > 101888 {
		t.Fatalf("len=%d", len(text))
	}
}

func TestCatalog_SummaryOnlyNumbers(t *testing.T) {
	for k, v := range NewSpecCatalog(mkItems(3), 0).Result().Summary {
		if _, ok := v.(int); !ok {
			t.Errorf("%s bukan int", k)
		}
	}
}

func TestCatalog_Swagger2Label(t *testing.T) {
	resp := listspecs.Response{Items: []listspecs.Spec{
		{Name: "petstore", Title: "Swagger Petstore", ApiVersion: "1.0.0", OpenapiVersion: "2.0", EndpointCount: 4, UpdatedAt: ts},
		{Name: "billing", Title: "B", ApiVersion: "1", OpenapiVersion: "3.0.3", EndpointCount: 2, UpdatedAt: ts},
		{Name: "legacy", OpenapiVersion: ""},
	}}
	text := NewSpecCatalog(resp, 5000).Text()
	for _, w := range []string{
		"- petstore: Swagger Petstore | versi 1.0.0 | Swagger 2.0 | 4 endpoint | diperbarui 2026-10-20 08:15 UTC",
		"| OpenAPI 3.0.3 |",
		"| OpenAPI - |",
	} {
		if !strings.Contains(text, w) {
			t.Fatalf("tidak ada %q di:\n%s", w, text)
		}
	}
	long := listspecs.Response{Items: []listspecs.Spec{{Name: "x", OpenapiVersion: strings.Repeat("9", 100)}}}
	if got := NewSpecCatalog(long, 5000).Text(); !strings.Contains(got, "| OpenAPI 9") || !strings.Contains(got, "... |") {
		t.Fatal(got)
	}
}
