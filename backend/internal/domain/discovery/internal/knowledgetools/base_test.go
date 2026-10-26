package knowledgetools

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

var update = flag.Bool("update", false, "tulis ulang testdata/tools_list.golden.json")

func newTools(t *testing.T, maxBytes int) []toolkit.Tool {
	t.Helper()
	var d app.Deps
	d.Config.MCPMaxResponseBytes = maxBytes
	d.Bus = bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	tools := Register(d)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func TestRegister_ThreeReadOnlyTools(t *testing.T) {
	tools := newTools(t, 102400)
	if len(tools) != 3 {
		t.Fatalf("jumlah tool = %d, want 3", len(tools))
	}
	want := []string{"list_files", "read_file", "search_knowledge"}
	seen := map[string]bool{}
	for i, tl := range tools {
		if tl.Name != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, tl.Name, want[i])
		}
		if seen[tl.Name] {
			t.Errorf("nama duplikat %q", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Handler == nil {
			t.Errorf("%s: Handler nil", tl.Name)
		}
		if tl.Title == "" || tl.Description == "" {
			t.Errorf("%s: Title/Description kosong", tl.Name)
		}
		if !json.Valid(tl.InputSchema) {
			t.Errorf("%s: InputSchema bukan JSON valid", tl.Name)
		}
	}
}

func TestT08_AC6_ToolsListContract(t *testing.T) {
	tools := newTools(t, 102400)
	type entry struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		InputSchema any    `json:"inputSchema"`
	}
	out := make([]entry, 0, len(tools))
	for _, tl := range tools {
		var schema any
		if err := json.Unmarshal(tl.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tl.Name, err)
		}
		out = append(out, entry{tl.Name, tl.Title, tl.Description, schema})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "tools_list.golden.json")
	if *update {
		if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Errorf("tools/list berbeda dari golden; jalankan -update lalu periksa diff.\ngot:\n%s", buf.String())
	}
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func validateSchema(t *testing.T, name string, raw []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if root["type"] != "object" {
		t.Errorf("%s: root type != object", name)
	}
	if root["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties harus false", name)
	}
	for k := range root {
		switch k {
		case "type", "properties", "required", "additionalProperties":
		default:
			t.Errorf("%s: keyword root terlarang %q", name, k)
		}
	}
	props, _ := root["properties"].(map[string]any)
	if len(props) == 0 {
		t.Errorf("%s: properties kosong", name)
	}
	for _, banned := range []string{"project", "project_id", "projectId"} {
		if _, ok := props[banned]; ok {
			t.Errorf("%s: properti terlarang %q", name, banned)
		}
	}
	allowed := map[string]bool{"type": true, "description": true, "default": true, "minimum": true, "maximum": true, "minLength": true, "maxLength": true}
	for pn, pv := range props {
		p, _ := pv.(map[string]any)
		typ, _ := p["type"].(string)
		if typ != "string" && typ != "integer" && typ != "boolean" {
			t.Errorf("%s.%s: type %q tidak didukung", name, pn, typ)
		}
		if d, _ := p["description"].(string); d == "" {
			t.Errorf("%s.%s: description kosong", name, pn)
		}
		for k := range p {
			if !allowed[k] {
				t.Errorf("%s.%s: keyword terlarang %q", name, pn, k)
			}
		}
		for _, k := range []string{"minimum", "maximum"} {
			if _, ok := p[k]; ok && typ != "integer" {
				t.Errorf("%s.%s: %s hanya untuk integer", name, pn, k)
			}
		}
		for _, k := range []string{"minLength", "maxLength"} {
			if _, ok := p[k]; ok && typ != "string" {
				t.Errorf("%s.%s: %s hanya untuk string", name, pn, k)
			}
		}
		if def, ok := p["default"]; ok {
			switch typ {
			case "string":
				if _, ok := def.(string); !ok {
					t.Errorf("%s.%s: default bukan string", name, pn)
				}
			case "boolean":
				if _, ok := def.(bool); !ok {
					t.Errorf("%s.%s: default bukan boolean", name, pn)
				}
			case "integer":
				f, ok := def.(float64)
				if !ok || f != float64(int64(f)) {
					t.Errorf("%s.%s: default bukan integer", name, pn)
					break
				}
				if mn, ok := p["minimum"].(float64); ok && f < mn {
					t.Errorf("%s.%s: default < minimum", name, pn)
				}
				if mx, ok := p["maximum"].(float64); ok && f > mx {
					t.Errorf("%s.%s: default > maximum", name, pn)
				}
			}
		}
		if mn, ok := p["minimum"].(float64); ok {
			if mx, ok := p["maximum"].(float64); ok && mn > mx {
				t.Errorf("%s.%s: minimum > maximum", name, pn)
			}
		}
	}
	if req, ok := root["required"].([]any); ok {
		for _, r := range req {
			if _, ok := props[r.(string)]; !ok {
				t.Errorf("%s: required %v tidak ada di properties", name, r)
			}
		}
	}
	return props
}

func TestT08_AC6_SchemasAreValidJSONSchema(t *testing.T) {
	actions := map[string]bool{}
	for _, a := range record.Actions() {
		actions[a] = true
	}
	for _, tl := range newTools(t, 102400) {
		validateSchema(t, tl.Name, tl.InputSchema)
		if !nameRe.MatchString(tl.Name) {
			t.Errorf("nama tool %q tidak valid", tl.Name)
		}
		if !actions["mcp."+tl.Name] {
			t.Errorf("aksi audit mcp.%s tidak terdaftar", tl.Name)
		}
	}
}

func TestT08_AC6_ReadFileLimitFollowsConfig(t *testing.T) {
	for _, tl := range newTools(t, 1024) {
		if tl.Name != "read_file" {
			continue
		}
		props := validateSchema(t, tl.Name, tl.InputSchema)
		lim := props["limit"].(map[string]any)
		if lim["maximum"] != float64(1024) || lim["default"] != float64(1024) {
			t.Errorf("limit = %v, want maximum=default=1024", lim)
		}
		return
	}
	t.Fatal("read_file tidak ditemukan")
}

func TestT08_AC7_ImportAllowlist(t *testing.T) {
	exact := map[string]bool{
		"grimoire/internal/infra/app":                         true,
		"grimoire/internal/infra/bus":                         true,
		"grimoire/lib/constants":                              true,
		"grimoire/lib/contracts/project/knowledge/listfiles":  true,
		"grimoire/lib/contracts/project/knowledge/readfile":   true,
		"grimoire/lib/contracts/project/knowledge/search":     true,
		"grimoire/lib/contracts/discovery/mcpgateway/toolkit": true,
		"grimoire/lib/common/errs":                            true,
		// T11: batas audit, kontrak scan audit, dan kontrak laporan kualitas pencarian.
		"grimoire/lib/contracts/core/audit/record":                      true,
		"grimoire/lib/contracts/core/audit/scanentries":                 true,
		"grimoire/lib/contracts/discovery/knowledgetools/searchquality": true,
		"grimoire/lib/util/mdoutline":                                   true, // T18
	}
	const self = "grimoire/internal/domain/discovery/internal/knowledgetools"
	const recordPkg = "grimoire/lib/contracts/core/audit/record"
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(".", func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, im := range f.Imports {
			ip, _ := strconv.Unquote(im.Path.Value)
			first := strings.SplitN(ip, "/", 2)[0]
			switch {
			case !strings.Contains(first, ".") && first != "grimoire":
				// stdlib
			case exact[ip], ip == self, strings.HasPrefix(ip, self+"/"):
			case (ip == recordPkg || ip == "grimoire/internal/infra/config") && strings.HasSuffix(p, "_test.go"):
				// khusus tes: record (AC6) dan config (nilai Config untuk Deps)
			default:
				t.Errorf("%s: import terlarang %s", p, ip)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Errorf("hanya %d file diperiksa", checked)
	}
}
