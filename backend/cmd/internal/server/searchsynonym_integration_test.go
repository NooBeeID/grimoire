//go:build integration

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ssAuditExpanded membaca entri audit mcp.search_knowledge terbaru; menunggu entri ke-n muncul (audit async).
func ssAuditExpanded(t *testing.T, e *mcpEnv, wantCount int) bool {
	t.Helper()
	for i := 0; i < 50; i++ {
		if e.countAudit("mcp.search_knowledge") >= wantCount {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	var result []byte
	q := "SELECT result FROM " + e.schema + ".audit_log WHERE action = 'mcp.search_knowledge' ORDER BY occurred_at DESC, id DESC LIMIT 1"
	if err := e.admin.QueryRow(q).Scan(&result); err != nil {
		t.Fatalf("baca audit: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatal(err)
	}
	v, _ := res["expanded"].(bool)
	return v
}

func TestT20_SynonymViaAPI(t *testing.T) {
	e := seSetupMCP(t)
	e.seedKnowledge(e.projID, "auth/autentikasi.md", "# Autentikasi\n\nAlur autentikasi pengguna lewat token sesi.\n")
	base := "/api/projects/" + e.projID + "/search/synonyms"

	if got := seSearch(t, e, "login"); len(got) != 0 {
		t.Fatalf("sebelum kamus: %v, ingin kosong", got)
	}

	resp, b := e.rest(http.MethodPost, base, map[string]any{"terms": []string{"login", "autentikasi"}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var g struct {
		ID    string   `json:"id"`
		Terms []string `json:"terms"`
	}
	if err := json.Unmarshal(b, &g); err != nil || g.ID == "" {
		t.Fatalf("respons create: %v %s", err, b)
	}

	got := seSearch(t, e, "login")
	if len(got) == 0 || got[0] != "auth/autentikasi.md" {
		t.Fatalf("setelah kamus: %v, ingin item pertama auth/autentikasi.md", got)
	}
	if !ssAuditExpanded(t, e, 2) {
		t.Error("audit mcp.search_knowledge terbaru: result.expanded harus true")
	}

	if resp, b := e.rest(http.MethodDelete, base+"/"+g.ID, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", resp.StatusCode, b)
	}
	if got := seSearch(t, e, "login"); len(got) != 0 {
		t.Fatalf("setelah hapus kelompok: %v, ingin kosong", got)
	}
}

func TestT20_SynonymAuthz(t *testing.T) {
	e := seSetupMCP(t)
	base := "/api/projects/" + e.projID + "/search/synonyms"

	// Pengguna developer yang bukan anggota project.
	if resp, b := e.rest(http.MethodPost, "/api/users", map[string]string{
		"email": "luar@tim.id", "name": "Luar", "role": "developer", "password": "dev-passw0rd-long"}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("buat user: %d %s", resp.StatusCode, b)
	}
	outsider := sumLogin(t, e.base, "luar@tim.id", "dev-passw0rd-long")
	const newPass = "dev-baru-passw0rd-long"
	if resp, b := sumDo(t, outsider, e.base, http.MethodPost, "/api/auth/change-password",
		map[string]string{"current_password": "dev-passw0rd-long", "new_password": newPass}); resp.StatusCode >= 300 {
		t.Fatalf("change-password: %d %s", resp.StatusCode, b)
	}
	outsider = sumLogin(t, e.base, "luar@tim.id", newPass)

	resp, b := sumDo(t, outsider, e.base, http.MethodGet, base, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET non-anggota: %d %s, ingin 404", resp.StatusCode, b)
	}
	resp, b = sumDo(t, outsider, e.base, http.MethodPost, base, map[string]any{"terms": []string{"a b", "c d"}})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST non-anggota: %d %s, ingin 404", resp.StatusCode, b)
	}

	// Tanpa header CSRF dari sesi admin -> 403.
	req, _ := http.NewRequest(http.MethodPost, e.base+base, bytes.NewReader([]byte(`{"terms":["login","masuk"]}`)))
	req.Header.Set("Content-Type", "application/json")
	cresp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cresp.Body.Close()
	if cresp.StatusCode != http.StatusForbidden {
		t.Errorf("POST tanpa CSRF: %d, ingin 403", cresp.StatusCode)
	}

	// Kamus tidak boleh bisa ditulis lewat MCP.
	resp2, body := e.mcp(e.rawKey, listBody, nil)
	if resp2.StatusCode != 200 {
		t.Fatalf("tools/list: %d %s", resp2.StatusCode, body)
	}
	var tl struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &tl); err != nil {
		t.Fatal(err)
	}
	if len(tl.Result.Tools) == 0 {
		t.Fatal("tools/list kosong")
	}
	for _, tool := range tl.Result.Tools {
		if n := strings.ToLower(tool.Name); strings.Contains(n, "synonym") || strings.Contains(n, "sinonim") {
			t.Errorf("tool menulis/mengelola sinonim terekspos di MCP: %s", tool.Name)
		}
	}
}
