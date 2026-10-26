package readsection

import (
	"encoding/json"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

func TestParse_Defaults(t *testing.T) {
	max := base.MaxReadLimit(102400)
	got, e := Parse(json.RawMessage(`{"path":"/a.md","heading":" Setup "}`), max)
	want := Params{"a.md", "Setup", 1, 40000}
	if e != nil || got != want {
		t.Fatalf("got %+v err=%v", got, e)
	}
	got, e = Parse(json.RawMessage(`{"path":"a.md","heading":"x","occurrence":1000,"limit":101888}`), max)
	if e != nil || got.Occurrence != 1000 || got.Limit != 101888 {
		t.Fatalf("got %+v err=%v", got, e)
	}
	if max != 101888 {
		t.Fatalf("max = %d", max)
	}
}

func TestParse_Invalid(t *testing.T) {
	max := base.MaxReadLimit(102400)
	cases := []struct{ name, in, want string }{
		{"heading hilang", `{"path":"a.md"}`, "heading wajib diisi"},
		{"heading kosong", `{"path":"a.md","heading":"  "}`, "heading wajib diisi"},
		{"heading panjang", `{"path":"a.md","heading":"` + strings.Repeat("x", 501) + `"}`, "heading maksimal 500"},
		{"path hilang", `{"heading":"x"}`, "path wajib diisi"},
		{"occurrence 0", `{"path":"a.md","heading":"x","occurrence":0}`, "occurrence harus 1-1000"},
		{"occurrence 1001", `{"path":"a.md","heading":"x","occurrence":1001}`, "occurrence harus 1-1000"},
		{"occurrence string", `{"path":"a.md","heading":"x","occurrence":"1"}`, "harus integer"},
		{"limit 0", `{"path":"a.md","heading":"x","limit":0}`, "limit harus 1-101888"},
		{"limit max+1", `{"path":"a.md","heading":"x","limit":101889}`, "limit harus 1-101888"},
		{"project_id", `{"path":"a.md","heading":"x","project_id":"p"}`, "ditentukan oleh API key"},
		{"projectId", `{"path":"a.md","heading":"x","projectId":"p"}`, "ditentukan oleh API key"},
		{"project", `{"path":"a.md","heading":"x","project":"p"}`, "ditentukan oleh API key"},
		{"asing", `{"path":"a.md","heading":"x","foo":1}`, "path, heading, occurrence, limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, e := Parse(json.RawMessage(c.in), max)
			if e == nil || !strings.Contains(e.Message(), c.want) {
				t.Fatalf("err = %v, want memuat %q", e, c.want)
			}
		})
	}
}
