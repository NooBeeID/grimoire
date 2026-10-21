package openapi

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func formatOf(name string) Format {
	if filepath.Ext(name) == ".json" {
		return FormatJSON
	}
	return FormatYAML
}

func parseFile(t testing.TB, name string) (Document, []Issue) {
	t.Helper()
	return Parse(load(t, name), formatOf(name), DefaultLimits)
}

func mustParse(t testing.TB, name string) Document {
	t.Helper()
	doc, is := parseFile(t, name)
	if len(is) > 0 {
		t.Fatalf("%s: unexpected issues %+v", name, is)
	}
	return doc
}

func codes(is []Issue) []string {
	var out []string
	for _, i := range is {
		out = append(out, i.Code)
	}
	return out
}
