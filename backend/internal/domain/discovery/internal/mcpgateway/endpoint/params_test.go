package endpoint

import (
	"encoding/json"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func TestParseInitializeParams(t *testing.T) {
	long := strings.Repeat("a", 200)
	tests := []struct {
		name, in, reason       string
		version, cname, cversn string
	}{
		{"valid penuh", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"cc","version":"2.1"}}`, "", "2025-06-18", "cc", "2.1"},
		{"tanpa clientInfo", `{"protocolVersion":"2025-06-18"}`, "", "2025-06-18", "", ""},
		{"name angka", `{"protocolVersion":"x","clientInfo":{"name":5,"version":"1"}}`, "", "x", "", "1"},
		{"nama dipotong", `{"protocolVersion":"x","clientInfo":{"name":"` + long + `"}}`, "", "x", long[:128], ""},
		{"kosong", ``, "params must be an object", "", "", ""},
		{"array", `[]`, "params must be an object", "", "", ""},
		{"string", `"x"`, "params must be an object", "", "", ""},
		{"version hilang", `{}`, "protocolVersion must be a string", "", "", ""},
		{"version angka", `{"protocolVersion":1}`, "protocolVersion must be a string", "", "", ""},
		{"version null", `{"protocolVersion":null}`, "protocolVersion must be a string", "", "", ""},
		{"version kosong lolos", `{"protocolVersion":""}`, "", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, pe := parseInitializeParams(json.RawMessage(tc.in))
			if tc.reason != "" {
				if pe == nil || pe.Reason != tc.reason {
					t.Fatalf("pe = %v", pe)
				}
				return
			}
			if pe != nil {
				t.Fatalf("pe = %v", pe)
			}
			if got.ProtocolVersion != tc.version || got.ClientName != tc.cname || got.ClientVersion != tc.cversn {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestParseToolsCallParams(t *testing.T) {
	n128 := strings.Repeat("a", 128)
	multi := strings.Repeat("é", 65) // 130 byte
	tests := []struct {
		name, in, reason, args string
	}{
		{"tanpa arguments", `{"name":"read_file"}`, "", `{}`},
		{"arguments persis", `{"name":"read_file","arguments": {"path" : "a"}}`, "", `{"path" : "a"}`},
		{"project_id utuh", `{"name":"x","arguments":{"project_id":"B"}}`, "", `{"project_id":"B"}`},
		{"name 128", `{"name":"` + n128 + `"}`, "", `{}`},
		{"name 129", `{"name":"` + n128 + `a"}`, "name too long (max 128 bytes)", ""},
		{"name multibyte", `{"name":"` + multi + `"}`, "name too long (max 128 bytes)", ""},
		{"name hilang", `{}`, "name must be a string", ""},
		{"name angka", `{"name":1}`, "name must be a string", ""},
		{"name kosong", `{"name":""}`, "name must not be empty", ""},
		{"arguments null", `{"name":"x","arguments":null}`, "arguments must be an object", ""},
		{"arguments array", `{"name":"x","arguments":[]}`, "arguments must be an object", ""},
		{"arguments string", `{"name":"x","arguments":"s"}`, "arguments must be an object", ""},
		{"arguments angka", `{"name":"x","arguments":1}`, "arguments must be an object", ""},
		{"params hilang", ``, "params must be an object", ""},
		{"params array", `[]`, "params must be an object", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, pe := parseToolsCallParams(json.RawMessage(tc.in))
			if tc.reason != "" {
				if pe == nil || pe.Reason != tc.reason {
					t.Fatalf("pe = %v", pe)
				}
				return
			}
			if pe != nil {
				t.Fatalf("pe = %v", pe)
			}
			if string(got.Arguments) != tc.args {
				t.Fatalf("args = %s", got.Arguments)
			}
		})
	}
}

func TestToToolEntries(t *testing.T) {
	b, _ := json.Marshal(toolsListResult{Tools: toToolEntries(nil)})
	if string(b) != `{"tools":[]}` {
		t.Fatalf("%s", b)
	}
	schema := json.RawMessage(`{"type":"object","x":1}`)
	b, _ = json.Marshal(toToolEntries([]toolkit.Tool{
		{Name: "a", Description: "d", InputSchema: schema},
		{Name: "b", Title: "T", InputSchema: schema},
	}))
	s := string(b)
	if strings.Count(s, `"readOnlyHint":true`) != 2 || strings.Count(s, `"title"`) != 1 || !strings.Contains(s, string(schema)) {
		t.Fatalf("%s", s)
	}
	if strings.Index(s, `"a"`) > strings.Index(s, `"b"`) {
		t.Fatal("urutan berubah")
	}
}

func TestToCallToolResult(t *testing.T) {
	marshal := func(o base.Outcome) string {
		b, _ := json.Marshal(toCallToolResult(o))
		return string(b)
	}
	var ok base.Outcome
	ok.Result.Text = "hi"
	if got := marshal(ok); got != `{"content":[{"type":"text","text":"hi"}],"isError":false}` {
		t.Fatal(got)
	}
	var e base.Outcome
	e.Result.IsError = true
	if !strings.Contains(marshal(e), `"isError":true`) {
		t.Fatal("IsError")
	}
	to := base.Outcome{ErrorCode: "timeout"}
	if !strings.Contains(marshal(to), `"isError":true`) {
		t.Fatal("ErrorCode")
	}
	tr := base.Outcome{Truncated: true, OriginalBytes: 512}
	if !strings.Contains(marshal(tr), `"_meta":{"truncated":true,"original_bytes":512}`) {
		t.Fatal(marshal(tr))
	}
	if !strings.Contains(marshal(base.Outcome{}), `"text":""`) || strings.Contains(marshal(ok), "_meta") {
		t.Fatal("kosong / _meta")
	}
}
