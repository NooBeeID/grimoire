package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	// .../backend/internal/infra/config/docs_test.go -> repo root
	return filepath.Join(filepath.Dir(f), "..", "..", "..", "..", rel)
}

func envTags(t *testing.T) []string {
	t.Helper()
	rt := reflect.TypeOf(Config{})
	seen := map[string]bool{}
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("env")
		if tag == "" || seen[tag] {
			t.Fatalf("field %s: env tag missing or duplicate", rt.Field(i).Name)
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func TestConfig_AllFieldsHaveEnvTag(t *testing.T) { envTags(t) }

func docRows(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(repoFile(t, "docs/configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "| `") {
			name := l[3 : strings.Index(l[3:], "`")+3]
			rows[name] = l
		}
	}
	return rows
}

func TestT00_AC7_AllConfigFieldsDocumented(t *testing.T) {
	rows := docRows(t)
	c, err := Load(env(base(nil)))
	if err != nil {
		t.Fatal(err)
	}
	rv := reflect.ValueOf(c)
	rt := rv.Type()
	var missing []string
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Tag.Get("env")
		row, ok := rows[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if name == "DATABASE_URL" || name == "SESSION_SECRET" {
			continue // wajib, tanpa default
		}
		def := "-"
		if v := rv.Field(i); !v.IsZero() || v.Kind() == reflect.Bool {
			def = "`" + formatDefault(v) + "`"
		}
		if !strings.Contains(row, "| "+def+" |") {
			missing = append(missing, name+" (default "+def+")")
		}
	}
	if len(missing) > 0 {
		t.Fatalf("tidak terdokumentasi/ default beda: %v", missing)
	}
}

func formatDefault(v reflect.Value) string {
	if d, ok := v.Interface().(interface{ String() string }); ok {
		s := d.String()
		return strings.NewReplacer("m0s", "m", "h0m", "h").Replace(s)
	}
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	}
	return v.String()
}

func TestT00_AC7_NoUnknownEnvInDocsTable(t *testing.T) {
	known := map[string]bool{}
	for _, n := range envTags(t) {
		known[n] = true
	}
	for n := range docRows(t) {
		if !known[n] {
			t.Errorf("docs menyebut env tak dikenal: %s", n)
		}
	}
}
