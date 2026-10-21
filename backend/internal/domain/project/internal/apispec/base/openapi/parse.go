package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Swagger2Version adalah nilai OpenAPIVersion untuk dokumen Swagger 2.0.
const Swagger2Version = "2.0"

// Document adalah hasil parse dokumen OpenAPI/Swagger yang valid.
type Document struct {
	OpenAPIVersion string // "3.0.3" / "3.1.0" / "2.0" (Swagger)
	Title          string
	APIVersion     string
	Root           *Map
	node           *yaml.Node // untuk lookup baris (kosong untuk JSON tanpa issue)
}

// IsSwagger2 true bila dokumen berformat Swagger 2.0.
func (d Document) IsSwagger2() bool { return d.OpenAPIVersion == Swagger2Version }

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripBOM membuang UTF-8 BOM di awal (D8). Dipanggil usecase sebelum Parse/hash/simpan.
func StripBOM(b []byte) []byte { return bytes.TrimPrefix(b, utf8BOM) }

var (
	versionRe  = regexp.MustCompile(`^3\.(0|1)\.\d+$`)
	yamlLineRe = regexp.MustCompile(`line (\d+)`)
)

// Parse memeriksa dan mem-parse dokumen. len(issues) > 0 berarti ditolak (maksimal MaxIssues).
// Berhenti pada langkah pertama yang menghasilkan issue, kecuali validasi struktur dan $ref
// yang mengumpulkan semuanya (diurut per baris, dipotong MaxIssues).
func Parse(content []byte, format Format, lim Limits) (Document, []Issue) {
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return Document{}, []Issue{compose(CodeInvalidEncoding, "file bukan teks UTF-8 yang valid", "", 0)}
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return Document{}, []Issue{compose(CodeEmpty, "file kosong", "", 0)}
	}

	var (
		rootVal any
		node    *yaml.Node
		is      *Issue
	)
	if format == FormatJSON {
		rootVal, is = parseJSON(content, lim)
	} else {
		rootVal, node, is = parseYAML(content, lim)
	}
	if is != nil {
		return Document{}, []Issue{*is}
	}

	root, ok := asMap(rootVal)
	if !ok {
		return Document{}, []Issue{compose(CodeNotOpenAPI, "bukan dokumen OpenAPI", "#", 1)}
	}
	ver, has := root.Get("openapi")
	if !has {
		sw, hasSwagger := root.Get("swagger")
		if !hasSwagger || sw == nil {
			return Document{}, []Issue{compose(CodeNotOpenAPI, "bukan dokumen OpenAPI", "#", 1)}
		}
		sv, ok, shown := swaggerVersion(sw)
		if !ok {
			return Document{}, []Issue{compose(CodeUnsupportedVersion,
				"versi Swagger "+strconv.Quote(truncRunes(shown, 40))+" tidak didukung (hanya 2.0)",
				"#/swagger", lineOfPointer(content, format, node, "#/swagger"))}
		}
		doc := Document{OpenAPIVersion: sv, Root: root, node: node}
		raws := validateSwagger2(doc)
		raws = append(raws, checkRefs(doc)...)
		return finish(doc, raws, content)
	}
	vs, isStr := ver.(string)
	if !isStr || !versionRe.MatchString(vs) {
		return Document{}, []Issue{compose(CodeUnsupportedVersion,
			"versi OpenAPI "+strconv.Quote(truncRunes(versionShown(ver), 40))+" tidak didukung (hanya 3.0.x/3.1.x)",
			"#/openapi", lineOfPointer(content, format, node, "#/openapi"))}
	}

	doc := Document{OpenAPIVersion: vs, Root: root, node: node}
	raws := validate(doc)
	raws = append(raws, checkRefs(doc)...)
	return finish(doc, raws, content)
}

// finish mengubah raw issue menjadi Issue terurut (ditolak) atau mengisi Title/APIVersion dari info.
func finish(doc Document, raws []rawIssue, content []byte) (Document, []Issue) {
	if len(raws) > 0 {
		node := doc.node
		if node == nil {
			node = jsonLineNode(content)
		}
		out := make([]Issue, 0, len(raws))
		for _, r := range raws {
			out = append(out, compose(r.code, r.msg, r.pointer, nodeLine(node, r.pointer)))
		}
		return Document{}, sortIssues(out)
	}
	if info, ok := asMap(mustGet(doc.Root, "info")); ok {
		doc.Title, _ = mustGet(info, "title").(string)
		doc.APIVersion, _ = mustGet(info, "version").(string)
	}
	return doc, nil
}

// versionShown membentuk teks versi untuk pesan penolakan.
func versionShown(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return "?"
}

// swaggerVersion menerima "2.0" (string), 2.0 (float64), 2 / 2.0 (json.Number); semuanya menjadi "2.0".
// Nilai lain -> ok=false dan shown untuk pesan.
func swaggerVersion(v any) (ver string, ok bool, shown string) {
	switch x := v.(type) {
	case string:
		if x == Swagger2Version {
			return Swagger2Version, true, x
		}
	case float64:
		if x == 2 {
			return Swagger2Version, true, ""
		}
	case json.Number:
		if s := x.String(); s == "2" || s == Swagger2Version {
			return Swagger2Version, true, s
		}
	}
	return "", false, versionShown(v)
}

func mustGet(m *Map, k string) any {
	v, _ := m.Get(k)
	return v
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func lineOfPointer(content []byte, format Format, node *yaml.Node, ptr string) int {
	if node == nil && format == FormatJSON {
		node = jsonLineNode(content)
	}
	return nodeLine(node, ptr)
}

// jsonLineNode mem-parse JSON sebagai YAML hanya untuk mendapat nomor baris; gagal -> nil.
func jsonLineNode(content []byte) *yaml.Node {
	var n yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(content)).Decode(&n); err != nil {
		return nil
	}
	return &n
}

// ---------- JSON ----------

func lineAtOffset(content []byte, off int64) int {
	if off > int64(len(content)) {
		off = int64(len(content))
	}
	if off < 0 {
		off = 0
	}
	return bytes.Count(content[:off], []byte{'\n'}) + 1
}

func jsonSyntaxIssue(content []byte, err error) Issue {
	line := 1
	var se *json.SyntaxError
	switch {
	case errors.As(err, &se):
		line = lineAtOffset(content, se.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		line = lineAtOffset(content, int64(len(content)))
	}
	return compose(CodeSyntax, "JSON tidak valid", "", line)
}

type jsonBuilder struct {
	dec *json.Decoder
	lim Limits
	err *Issue
}

func (b *jsonBuilder) limit() *Issue {
	is := compose(CodeYAMLLimit, "dokumen YAML terlalu kompleks (alias/kedalaman)", "", 0)
	return &is
}

func (b *jsonBuilder) value(tok json.Token, depth int) any {
	if b.err != nil {
		return nil
	}
	// JSON tidak punya alias, jadi jumlah node dibatasi ukuran file (batas pemanggil); hanya kedalaman dicek.
	if depth > b.lim.MaxDepth {
		b.err = b.limit()
		return nil
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			m := NewMap()
			for b.dec.More() {
				kt, err := b.dec.Token()
				if err != nil {
					b.fail()
					return nil
				}
				k, _ := kt.(string)
				vt, err := b.dec.Token()
				if err != nil {
					b.fail()
					return nil
				}
				if _, dup := m.Get(k); dup {
					is := compose(CodeSyntax, "kunci duplikat "+strconv.Quote(truncRunes(k, 60)), "", 0)
					b.err = &is
					return nil
				}
				m.Set(k, b.value(vt, depth+1))
				if b.err != nil {
					return nil
				}
			}
			if _, err := b.dec.Token(); err != nil { // '}'
				b.fail()
			}
			return m
		}
		arr := []any{}
		for b.dec.More() {
			vt, err := b.dec.Token()
			if err != nil {
				b.fail()
				return nil
			}
			arr = append(arr, b.value(vt, depth+1))
			if b.err != nil {
				return nil
			}
		}
		if _, err := b.dec.Token(); err != nil { // ']'
			b.fail()
		}
		return arr
	default:
		return tok // string, json.Number, bool, nil
	}
}

func (b *jsonBuilder) fail() {
	if b.err == nil {
		is := compose(CodeSyntax, "JSON tidak valid", "", 0)
		b.err = &is
	}
}

func parseJSON(content []byte, lim Limits) (any, *Issue) {
	if !json.Valid(content) {
		dec := json.NewDecoder(bytes.NewReader(content))
		var v any
		err := dec.Decode(&v)
		if err == nil {
			// nilai pertama valid, ada sisa yang tidak valid
			_, err = dec.Token()
			if err == nil || errors.Is(err, io.EOF) {
				var se = &json.SyntaxError{}
				se.Offset = dec.InputOffset()
				err = se
			}
		}
		is := jsonSyntaxIssue(content, err)
		return nil, &is
	}
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		is := jsonSyntaxIssue(content, err)
		return nil, &is
	}
	b := &jsonBuilder{dec: dec, lim: lim}
	v := b.value(tok, 1)
	if b.err != nil {
		return nil, b.err
	}
	return v, nil
}

// ---------- YAML ----------

func parseYAML(content []byte, lim Limits) (any, *yaml.Node, *Issue) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var n yaml.Node
	if err := dec.Decode(&n); err != nil {
		if errors.Is(err, io.EOF) {
			is := compose(CodeEmpty, "file kosong", "", 0)
			return nil, nil, &is
		}
		is := yamlSyntaxIssue(err)
		return nil, nil, &is
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		is := compose(CodeMultiDocument, "dokumen YAML lebih dari satu tidak didukung", "", extra.Line)
		return nil, nil, &is
	}
	if is := guard(&n, lim); is != nil {
		return nil, nil, is
	}
	c := &converter{memo: map[*yaml.Node]any{}}
	v := c.convert(&n)
	if c.err != nil {
		return nil, nil, c.err
	}
	return v, &n, nil
}

func yamlSyntaxIssue(err error) Issue {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 0
	if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
	}
	// buang awalan "line N: " dan baris tambahan (daftar unmarshal errors)
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = strings.TrimSpace(msg[i+1:])
	}
	if i := strings.Index(msg, ": "); i >= 0 && strings.HasPrefix(msg, "line ") {
		msg = msg[i+2:]
	}
	return compose(CodeSyntax, "YAML tidak valid: "+truncRunes(msg, 200), "", line)
}

type converter struct {
	memo map[*yaml.Node]any
	err  *Issue
}

func (c *converter) fail(code, msg string, line int) {
	if c.err == nil {
		is := compose(code, msg, "", line)
		c.err = &is
	}
}

func (c *converter) convert(n *yaml.Node) any {
	if c.err != nil || n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return c.convert(n.Content[0])
	case yaml.AliasNode:
		return c.convert(n.Alias)
	case yaml.ScalarNode:
		return scalarValue(n)
	}
	if v, ok := c.memo[n]; ok {
		return v
	}
	var out any
	switch n.Kind {
	case yaml.SequenceNode:
		arr := make([]any, 0, len(n.Content))
		for _, ch := range n.Content {
			arr = append(arr, c.convert(ch))
		}
		out = arr
	case yaml.MappingNode:
		out = c.mapping(n)
	}
	c.memo[n] = out
	return out
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func (c *converter) mapping(n *yaml.Node) *Map {
	m := NewMap()
	var merges []*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := deref(n.Content[i]), n.Content[i+1]
		if kn == nil || kn.Kind != yaml.ScalarNode {
			c.fail(CodeInvalidStructure, "kunci map harus berupa nilai skalar", n.Content[i].Line)
			return m
		}
		if kn.ShortTag() == "!!merge" {
			merges = append(merges, vn)
			continue
		}
		if _, dup := m.Get(kn.Value); dup {
			c.fail(CodeSyntax, "YAML tidak valid: kunci duplikat "+strconv.Quote(truncRunes(kn.Value, 60)), kn.Line)
			return m
		}
		m.Set(kn.Value, c.convert(vn))
	}
	for _, mv := range merges {
		srcs := []*yaml.Node{deref(mv)}
		if d := deref(mv); d != nil && d.Kind == yaml.SequenceNode {
			srcs = srcs[:0]
			for _, e := range d.Content {
				srcs = append(srcs, deref(e))
			}
		}
		for _, s := range srcs {
			if s == nil || s.Kind != yaml.MappingNode {
				c.fail(CodeInvalidStructure, "nilai merge key (<<) harus berupa map", n.Line)
				return m
			}
			sm, _ := c.convert(s).(*Map)
			for _, k := range sm.Keys() {
				if _, has := m.Get(k); !has {
					v, _ := sm.Get(k)
					m.Set(k, v)
				}
			}
		}
	}
	return m
}

var decIntRe = regexp.MustCompile(`^[+-]?\d+$`)

func scalarValue(n *yaml.Node) any {
	switch n.ShortTag() {
	case "!!null":
		return nil
	case "!!bool":
		return strings.EqualFold(n.Value, "true")
	case "!!int":
		v := strings.ReplaceAll(n.Value, "_", "")
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			return json.Number(strconv.FormatInt(i, 10))
		}
		if u, err := strconv.ParseUint(strings.TrimPrefix(v, "+"), 0, 64); err == nil {
			return json.Number(strconv.FormatUint(u, 10))
		}
		if decIntRe.MatchString(v) {
			return json.Number(strings.TrimPrefix(v, "+"))
		}
		return n.Value
	case "!!float":
		f, err := strconv.ParseFloat(strings.ReplaceAll(n.Value, "_", ""), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return n.Value
		}
		return f
	default:
		return n.Value
	}
}

// nodeLine mencari baris node untuk pointer; pointer tidak ditemukan -> baris node terdalam yang ketemu.
func nodeLine(root *yaml.Node, ptr string) int {
	if root == nil {
		return 0
	}
	cur := root
	if cur.Kind == yaml.DocumentNode && len(cur.Content) > 0 {
		cur = cur.Content[0]
	}
	line := cur.Line
	if !strings.HasPrefix(ptr, "#/") {
		return line
	}
	for _, tok := range strings.Split(ptr[2:], "/") {
		tok = unescapePointerToken(percentDecode(tok))
		cur = deref(cur)
		if cur == nil {
			return line
		}
		var next *yaml.Node
		var keyLine int
		switch cur.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(cur.Content); i += 2 {
				if k := deref(cur.Content[i]); k != nil && k.Value == tok {
					next, keyLine = cur.Content[i+1], cur.Content[i].Line
				}
			}
		case yaml.SequenceNode:
			if idx, err := strconv.Atoi(tok); err == nil && idx >= 0 && idx < len(cur.Content) {
				next, keyLine = cur.Content[idx], cur.Content[idx].Line
			}
		}
		if next == nil {
			return line
		}
		if keyLine > 0 {
			line = keyLine
		}
		cur = next
	}
	return line
}
