//go:build integration

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Fixture dan helper bersama tes integrasi T08 (overview 10.3). Tes per tool ada di
// knowledgetools_<slug>_integration_test.go.

// ktResult adalah hasil tools/call MCP yang diurai untuk asersi.
type ktResult struct {
	Text              string
	IsError           bool
	StructuredContent map[string]any
	Meta              map[string]any
	Raw               string
}

// mcpCall mengirim initialize lalu tools/call, dan mengurai hasilnya.
func (e *mcpEnv) mcpCall(key, name string, args any) ktResult {
	e.t.Helper()
	if resp, b := e.mcp(key, initBody, nil); resp.StatusCode != 200 {
		e.t.Fatalf("initialize: %d %s", resp.StatusCode, b)
	}
	ab, err := json.Marshal(args)
	if err != nil {
		e.t.Fatal(err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, ab)
	resp, raw := e.mcp(key, body, nil)
	if resp.StatusCode != 200 {
		e.t.Fatalf("tools/call %s: %d %s", name, resp.StatusCode, raw)
	}
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError           bool           `json:"isError"`
			StructuredContent map[string]any `json:"structuredContent"`
			Meta              map[string]any `json:"_meta"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		e.t.Fatalf("respons bukan JSON: %v: %s", err, raw)
	}
	if out.Error != nil {
		e.t.Fatalf("JSON-RPC error %d: %s", out.Error.Code, out.Error.Message)
	}
	var texts []string
	for _, c := range out.Result.Content {
		texts = append(texts, c.Text)
	}
	return ktResult{
		Text: strings.Join(texts, "\n"), IsError: out.Result.IsError,
		StructuredContent: out.Result.StructuredContent, Meta: out.Result.Meta, Raw: raw,
	}
}

// seedKnowledge membuat file knowledge lewat REST T05 pada project projID.
func (e *mcpEnv) seedKnowledge(projID, path, content string) {
	e.t.Helper()
	resp, b := e.rest(http.MethodPost, "/api/projects/"+projID+"/knowledge/files",
		map[string]string{"path": path, "content": content})
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("seed %s: %d %s", path, resp.StatusCode, b)
	}
}

// newProject membuat project tambahan dan mengembalikan id-nya.
func (e *mcpEnv) newProject(name string) string {
	e.t.Helper()
	resp, b := e.rest(http.MethodPost, "/api/projects", map[string]string{"name": name})
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("create project: %d %s", resp.StatusCode, b)
	}
	var pr struct{ ID string }
	if err := json.Unmarshal(b, &pr); err != nil {
		e.t.Fatal(err)
	}
	return pr.ID
}

// ktBigText membangun isi >= n byte berupa campuran ASCII dan multibyte 3/4 byte.
func ktBigText(n int) string {
	var sb strings.Builder
	unit := []string{"abcdefghij\n", "日本語テキスト\n", "emoji \U0001F600\U0001F680\n"}
	for i := 0; sb.Len() < n; i++ {
		sb.WriteString(unit[i%len(unit)])
	}
	return sb.String()
}

// ktEnv adalah fixture knowledge: project A (e.projID) dengan 5 file dan project B dengan SECRET-B.
type ktEnv struct {
	*mcpEnv
	projB string
	big   string
}

// setupKnowledgeTools menyiapkan MCP env beserta fixture overview 10.3.
func setupKnowledgeTools(t *testing.T) *ktEnv {
	t.Helper()
	e := setupMCP(t)
	k := &ktEnv{mcpEnv: e, big: ktBigText(200 * 1024)}
	e.seedKnowledge(e.projID, "readme.md", strings.Repeat("r", 100))
	e.seedKnowledge(e.projID, "docs/a.md", "# A\n\nisi a\n")
	e.seedKnowledge(e.projID, "docs/readme.md", "# Docs\n")
	e.seedKnowledge(e.projID, "docs/guides/b.md", "rate limit per key\n")
	e.seedKnowledge(e.projID, "docs/big.txt", k.big)
	k.projB = e.newProject("Other")
	e.seedKnowledge(k.projB, "secret/b.md", "SECRET-B")
	return k
}

// Memastikan fixture dan helper bersama bekerja terhadap server nyata (tes per tool ada di file lain).
func TestT08_Int_FixtureSeeded(t *testing.T) {
	k := setupKnowledgeTools(t)
	_, body := k.mcp(k.rawKey, listBody, nil)
	for _, name := range []string{"list_files", "read_file", "search_knowledge"} {
		if !strings.Contains(body, `"name":"`+name+`"`) {
			t.Fatalf("tool %s tidak ada: %s", name, body)
		}
	}
}
