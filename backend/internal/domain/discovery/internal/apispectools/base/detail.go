package base

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

const (
	maxMarkers      = 20  // per daftar
	markerBudget    = 700 // byte total string marker yang ditampilkan
	markerClipRunes = 100
	maxSimilar      = 5
	similarSummary  = 160
	suggestQueryMax = 40
	pathShownMax    = 200

	legendBase      = "$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth"
	legendTruncated = "; $truncated: operasi terlalu besar, hanya ringkasan"
)

// EndpointAsk adalah permintaan agent setelah normalisasi Parse.
type EndpointAsk struct {
	Spec, Method, Path, OperationID string // Method sudah uppercase; Path persis
}

// Lookup adalah hasil get_api_endpoint: EndpointDetail, EndpointMiss, atau SpecMiss.
type Lookup interface{ Result() toolkit.Result }

// NewLookup memilih entity dari resp. Error hanya untuk pelanggaran kontrak provider
// (Reason tak dikenal, atau JSON Operation/Servers/SecuritySchemes tidak valid).
func NewLookup(ask EndpointAsk, requestedDepth, maxBytes int, resp getendpoint.Response) (Lookup, error) {
	if resp.Found {
		if len(bytes.TrimSpace(resp.Operation)) == 0 || !json.Valid(resp.Operation) || bytes.Equal(bytes.TrimSpace(resp.Operation), []byte("null")) {
			return nil, fmt.Errorf("operation dari provider bukan JSON valid")
		}
		if len(bytes.TrimSpace(resp.Servers)) == 0 {
			resp.Servers = json.RawMessage("[]")
		} else if !json.Valid(resp.Servers) {
			return nil, fmt.Errorf("servers dari provider bukan JSON valid")
		}
		if len(bytes.TrimSpace(resp.SecuritySchemes)) == 0 {
			resp.SecuritySchemes = json.RawMessage("{}")
		} else if !json.Valid(resp.SecuritySchemes) {
			return nil, fmt.Errorf("securitySchemes dari provider bukan JSON valid")
		}
		if resp.OpenAPIVersion != "" {
			var err error
			if resp.Consumes, err = listOrEmpty(resp.Consumes, "consumes"); err != nil {
				return nil, err
			}
			if resp.Produces, err = listOrEmpty(resp.Produces, "produces"); err != nil {
				return nil, err
			}
		}
		return EndpointDetail{Resp: resp, RequestedDepth: requestedDepth, MaxBytes: maxBytes}, nil
	}
	switch resp.Reason {
	case getendpoint.ReasonSpecRequired:
		return SpecMiss{Tool: ToolGetAPIEndpoint, Requested: ask.Spec, Required: true, Valid: resp.Specs}, nil
	case getendpoint.ReasonSpecNotFound:
		return SpecMiss{Tool: ToolGetAPIEndpoint, Requested: ask.Spec, Required: false, Valid: resp.Specs}, nil
	case getendpoint.ReasonEndpointNotFound:
		spec := resp.Spec
		if spec == "" {
			spec = ask.Spec
		}
		sim := resp.Similar
		if len(sim) > maxSimilar {
			sim = sim[:maxSimilar]
		}
		return EndpointMiss{Spec: spec, Ask: ask, Similar: append([]getendpoint.EndpointRef(nil), sim...)}, nil
	}
	return nil, fmt.Errorf("reason tidak dikenal %q (found=false)", resp.Reason)
}

func listOrEmpty(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("[]"), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s dari provider bukan JSON valid", name)
	}
	return raw, nil
}

// EndpointDetail adalah endpoint yang ditemukan.
type EndpointDetail struct {
	Resp           getendpoint.Response
	RequestedDepth int
	MaxBytes       int // MaxBytes yang dikirim ke provider
}

type markersJSON struct {
	Circular  []string `json:"circular"`
	Collapsed []string `json:"collapsed"`
	Legend    string   `json:"legend"`
}

type detailJSON struct {
	Spec            string          `json:"spec"`
	Method          string          `json:"method"`
	Path            string          `json:"path"`
	SchemaDepth     int             `json:"schema_depth"`
	Truncated       bool            `json:"truncated"`
	Hint            string          `json:"hint,omitempty"`
	Markers         *markersJSON    `json:"markers,omitempty"`
	Operation       json.RawMessage `json:"operation"`
	Servers         json.RawMessage `json:"servers"`
	SecuritySchemes json.RawMessage `json:"securitySchemes"`
	OpenAPIVersion  string          `json:"openapi_version,omitempty"`
	Consumes        json.RawMessage `json:"consumes,omitempty"`
	Produces        json.RawMessage `json:"produces,omitempty"`
}

func (d EndpointDetail) rootTruncated() bool {
	var m map[string]any
	if err := json.Unmarshal(d.Resp.Operation, &m); err != nil {
		return false
	}
	return m["$truncated"] == true
}

func walkMarkers(v any, circ, coll map[string]struct{}) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok {
				switch k {
				case "$circular":
					circ[Clip(s, markerClipRunes)] = struct{}{}
				case "$ref":
					coll[Clip(s, markerClipRunes)] = struct{}{}
				}
				continue
			}
			walkMarkers(val, circ, coll)
		}
	case []any:
		for _, e := range x {
			walkMarkers(e, circ, coll)
		}
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// collect mengembalikan penanda penuh (unik, urut).
func (d EndpointDetail) collect() (circular, collapsed []string) {
	circ, coll := map[string]struct{}{}, map[string]struct{}{}
	for _, raw := range []json.RawMessage{d.Resp.Operation, d.Resp.Servers, d.Resp.SecuritySchemes} {
		var v any
		if err := json.Unmarshal(raw, &v); err == nil {
			walkMarkers(v, circ, coll)
		}
	}
	return sortedKeys(circ), sortedKeys(coll)
}

func markerBytes(a, b []string) int {
	n := 0
	for _, s := range a {
		n += len(s) + 3
	}
	for _, s := range b {
		n += len(s) + 3
	}
	return n
}

// Markers mengembalikan penanda yang dibatasi (G4).
func (d EndpointDetail) Markers() (circular, collapsed []string) {
	circular, collapsed = d.collect()
	if len(circular) > maxMarkers {
		circular = circular[:maxMarkers]
	}
	if len(collapsed) > maxMarkers {
		collapsed = collapsed[:maxMarkers]
	}
	for markerBytes(circular, collapsed) > markerBudget {
		if len(circular) >= len(collapsed) {
			circular = circular[:len(circular)-1]
		} else {
			collapsed = collapsed[:len(collapsed)-1]
		}
	}
	return circular, collapsed
}

// Hint kosong bila tidak terpotong.
func (d EndpointDetail) Hint() string {
	if !d.Resp.Truncated {
		return ""
	}
	eff, req := d.Resp.EffectiveDepth, d.RequestedDepth
	if eff == 0 {
		if d.rootTruncated() {
			return "Operasi terlalu besar bahkan tanpa membuka skema; hanya ringkasan yang ditampilkan. Gunakan list_api_endpoints untuk konteks dan minta potongan spec ke pemilik project."
		}
		return fmt.Sprintf("Respons dibatasi %d byte; skema tidak dibuka sama sekali (kedalaman 0) dan tampil sebagai {\"$ref\": ...}. Detail skema yang dirujuk tidak dapat ditampilkan lebih jauh dalam batas ukuran respons.", d.MaxBytes)
	}
	s := eff
	if s < 1 {
		s = 1
	}
	if s >= req && req > 1 {
		s = req - 1
	}
	return fmt.Sprintf("Respons melebihi batas %d byte; skema hanya dibuka sampai kedalaman %d (diminta %d). Bagian lebih dalam tampil sebagai {\"$ref\": ...}. Turunkan schema_depth (mis. %d) agar respons ringkas dan konsisten.", d.MaxBytes, eff, req, s)
}

// Text adalah JSON kompak satu baris.
func (d EndpointDetail) Text() string {
	out := detailJSON{
		Spec: d.Resp.Spec, Method: d.Resp.Method, Path: d.Resp.Path,
		SchemaDepth: d.Resp.EffectiveDepth, Truncated: d.Resp.Truncated, Hint: d.Hint(),
		Operation: d.Resp.Operation, Servers: d.Resp.Servers, SecuritySchemes: d.Resp.SecuritySchemes,
	}
	if d.Resp.OpenAPIVersion != "" {
		out.OpenAPIVersion, out.Consumes, out.Produces = d.Resp.OpenAPIVersion, d.Resp.Consumes, d.Resp.Produces
	}
	circ, coll := d.Markers()
	fullCirc, fullColl := d.collect()
	rootTrunc := d.rootTruncated()
	if len(fullCirc) > 0 || len(fullColl) > 0 || rootTrunc {
		if circ == nil {
			circ = []string{}
		}
		if coll == nil {
			coll = []string{}
		}
		legend := legendBase
		if rootTrunc {
			legend += legendTruncated
		}
		out.Markers = &markersJSON{Circular: circ, Collapsed: coll, Legend: legend}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return `{"error":"gagal menyusun hasil"}`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// Summary adalah metadata audit saja.
func (d EndpointDetail) Summary() map[string]any {
	circ, coll := d.collect()
	return map[string]any{
		"found":           true,
		"schema_depth":    d.Resp.EffectiveDepth,
		"requested_depth": d.RequestedDepth,
		"truncated":       d.Resp.Truncated,
		"circular_count":  len(circ),
		"collapsed_count": len(coll),
	}
}

// Result adalah hasil sukses.
func (d EndpointDetail) Result() toolkit.Result {
	return toolkit.Result{
		Text:       d.Text(),
		TargetType: TargetTypeEndpoint,
		Target:     fmt.Sprintf("%s %s %s", d.Resp.Spec, d.Resp.Method, d.Resp.Path),
		Summary:    d.Summary(),
	}
}

// EndpointMiss adalah endpoint tidak ketemu pada spec yang valid.
type EndpointMiss struct {
	Spec    string
	Ask     EndpointAsk
	Similar []getendpoint.EndpointRef // maks 5
}

// Subject adalah ringkasan permintaan untuk teks.
func (m EndpointMiss) Subject() string {
	if m.Ask.OperationID != "" {
		return "operation_id " + Clip(OneLine(m.Ask.OperationID), pathShownMax)
	}
	return m.Ask.Method + " " + Clip(OneLine(m.Ask.Path), pathShownMax)
}

// Target adalah target audit.
func (m EndpointMiss) Target() string {
	if m.Ask.OperationID != "" {
		return m.Spec + " operation_id=" + m.Ask.OperationID
	}
	return m.Spec + " " + m.Ask.Method + " " + m.Ask.Path
}

// SuggestQuery adalah kata kunci saran untuk list_api_endpoints.
func (m EndpointMiss) SuggestQuery() string {
	if m.Ask.OperationID != "" {
		return Clip(OneLine(m.Ask.OperationID), suggestQueryMax)
	}
	for _, seg := range strings.Split(m.Ask.Path, "/") {
		if seg != "" && !strings.HasPrefix(seg, "{") {
			return Clip(OneLine(seg), suggestQueryMax)
		}
	}
	return ""
}

// Text menyusun pesan tidak ketemu beserta saran.
func (m EndpointMiss) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Endpoint tidak ditemukan di spec %s: %s", m.Spec, m.Subject())
	if len(m.Similar) > 0 {
		b.WriteString("\nSimilar:")
		for _, s := range m.Similar {
			line := "- " + s.Method + " " + Clip(OneLine(s.Path), pathShownMax)
			if sum := Clip(OneLine(s.Summary), similarSummary); sum != "" {
				line += " — " + sum
			}
			if s.OperationID != "" {
				line += " [" + Clip(OneLine(s.OperationID), opIDMaxRunes) + "]"
			}
			b.WriteString("\n" + line)
		}
	}
	call := "list_api_endpoints(spec=" + strconv.Quote(m.Spec)
	if q := m.SuggestQuery(); q != "" {
		call += ", query=" + strconv.Quote(q)
	}
	call += ")"
	if len(m.Similar) > 0 {
		b.WriteString("\nGunakan salah satu di atas, atau " + call + " untuk melihat endpoint yang ada.")
	} else {
		b.WriteString("\nPeriksa ejaan path atau operation_id, atau gunakan " + call + " untuk melihat endpoint yang ada.")
	}
	return b.String()
}

// Summary adalah metadata audit saja.
func (m EndpointMiss) Summary() map[string]any {
	return map[string]any{"found": false, "error_code": "endpoint_not_found", "similar_count": len(m.Similar)}
}

// Result adalah hasil isError.
func (m EndpointMiss) Result() toolkit.Result {
	return toolkit.Result{Text: m.Text(), IsError: true, TargetType: TargetTypeEndpoint, Target: m.Target(), Summary: m.Summary()}
}
