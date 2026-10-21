package base

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func okTool(name string) toolkit.Tool {
	return toolkit.Tool{
		Name:        name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(context.Context, toolkit.Call) (toolkit.Result, error) {
			return toolkit.Result{}, nil
		},
	}
}

func TestCatalog_EmptyValid(t *testing.T) {
	c, err := NewCatalog(nil)
	if err != nil || c.Len() != 0 || len(c.Names()) != 0 {
		t.Fatalf("err=%v len=%d", err, c.Len())
	}
}

func TestCatalog_OrderIndependent(t *testing.T) {
	a, err := NewCatalog([]toolkit.Tool{okTool("search_knowledge"), okTool("read_file"), okTool("list_files")})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewCatalog([]toolkit.Tool{okTool("list_files"), okTool("search_knowledge"), okTool("read_file")})
	want := []string{"list_files", "read_file", "search_knowledge"}
	if !slices.Equal(a.Names(), want) || !slices.Equal(b.Names(), want) {
		t.Fatalf("a=%v b=%v", a.Names(), b.Names())
	}
	if _, ok := a.Lookup("read_file"); !ok {
		t.Fatal("lookup read_file")
	}
	if _, ok := a.Lookup("nope"); ok {
		t.Fatal("lookup nope")
	}
	l := a.List()
	l[0].Name = "x"
	if a.List()[0].Name == "x" {
		t.Fatal("List harus salinan")
	}
}

func TestT07_AC8_DuplicateToolName(t *testing.T) {
	_, err := NewCatalog([]toolkit.Tool{okTool("read_file"), okTool("read_file")})
	if err == nil || !strings.Contains(err.Error(), `"read_file"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestCatalog_Invalid(t *testing.T) {
	noHandler := okTool("read_file")
	noHandler.Handler = nil
	badSchema := func(s string) toolkit.Tool { t := okTool("read_file"); t.InputSchema = json.RawMessage(s); return t }
	cases := map[string]toolkit.Tool{
		"upper":       okTool("Read"),
		"digit-first": okTool("1abc"),
		"dash":        okTool("a-b"),
		"too-long":    okTool("a" + strings.Repeat("b", 64)),
		"reserved":    okTool("auth"),
		"no-action":   okTool("foo"),
		"nil-handler": noHandler,
		"array":       badSchema(`[]`),
		"wrong-type":  badSchema(`{"type":"string"}`),
		"no-type":     badSchema(`{}`),
		"broken":      badSchema(`{`),
		"empty":       badSchema(``),
	}
	for name, tool := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewCatalog([]toolkit.Tool{tool}); err == nil {
				t.Fatal("harus error")
			}
		})
	}
	for _, r := range ReservedToolNames {
		if _, err := NewCatalog([]toolkit.Tool{okTool(r)}); err == nil || !strings.Contains(err.Error(), "terlarang") {
			t.Errorf("%s: %v", r, err)
		}
	}
}
