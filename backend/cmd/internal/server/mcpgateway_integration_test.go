//go:build integration

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/createadmin"
)

type mcpEnv struct {
	t       *testing.T
	base    string
	admin   *sql.DB
	schema  string
	client  *http.Client
	rawKey  string
	keyID   string
	project string
	projID  string
}

func (e *mcpEnv) rest(method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "grimoire")
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// mcp mengirim POST /mcp. key kosong = tanpa header auth.
func (e *mcpEnv) mcp(key, body string, hdr map[string]string) (*http.Response, string) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.base+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (e *mcpEnv) countAudit(action string) int {
	e.t.Helper()
	var n int
	q := "SELECT count(*) FROM " + e.schema + ".audit_log WHERE action = $1"
	if err := e.admin.QueryRow(q, action).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func setupMCP(t *testing.T) *mcpEnv {
	t.Helper()
	dbURL, schema, admin := testDBURL(t)
	cfg := testCfg(t, dbURL, map[string]string{"RUN_MIGRATION": "true", "COOKIE_SECURE": "false"})
	depsCh := make(chan app.Deps, 1)
	r := startRun(t, cfg, WithAppHook(func(d app.Deps) { depsCh <- d }))
	d := <-depsCh

	const email, pw = "admin@tim.id", "passw0rd-long-enough"
	if _, err := bus.CallTyped[createadmin.Response](context.Background(), d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN,
		createadmin.Request{Email: email, Name: "Admin", Password: pw}); err != nil {
		t.Fatal(err)
	}
	jar := newJar()
	e := &mcpEnv{t: t, base: r.base, admin: admin, schema: schema, client: &http.Client{Jar: jar}}

	if resp, b := e.rest(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": pw}); resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, b)
	}
	resp, b := e.rest(http.MethodPost, "/api/projects", map[string]string{"name": "Payments"})
	if resp.StatusCode != 201 {
		t.Fatalf("create project: %d %s", resp.StatusCode, b)
	}
	var pr struct{ ID string }
	_ = json.Unmarshal(b, &pr)
	e.projID, e.project = pr.ID, "Payments"
	e.newKey("claude-laptop")
	return e
}

func (e *mcpEnv) newKey(name string) (rawKey, keyID string) {
	e.t.Helper()
	resp, b := e.rest(http.MethodPost, "/api/projects/"+e.projID+"/api-keys", map[string]string{"name": name})
	if resp.StatusCode != 201 {
		e.t.Fatalf("create key: %d %s", resp.StatusCode, b)
	}
	var kr struct {
		Key    struct{ ID string }
		RawKey string `json:"raw_key"`
	}
	_ = json.Unmarshal(b, &kr)
	e.rawKey, e.keyID = kr.RawKey, kr.Key.ID
	return kr.RawKey, kr.Key.ID
}

const (
	initBody  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"it","version":"1"}}}`
	listBody  = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	pingBody  = `{"jsonrpc":"2.0","id":3,"method":"ping"}`
	rejAction = "mcp.auth.rejected"
)

func TestT07_AC1_RealKeyInitializeAndToolsList(t *testing.T) {
	e := setupMCP(t)
	resp, body := e.mcp(e.rawKey, initBody, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"serverInfo":{"name":"grimoire"`) {
		t.Fatalf("initialize: %d %s", resp.StatusCode, body)
	}
	resp, body = e.mcp(e.rawKey, listBody, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"name":"list_files"`) {
		t.Fatalf("tools/list: %d %s", resp.StatusCode, body)
	}
	var actorType, actorID, label string
	q := "SELECT actor_type, actor_id::text, project_label FROM " + e.schema + ".audit_log WHERE action = 'mcp.initialize'"
	if err := e.admin.QueryRow(q).Scan(&actorType, &actorID, &label); err != nil {
		t.Fatal(err)
	}
	if actorType != "api_key" || actorID != e.keyID || label != e.project {
		t.Fatalf("audit: %s %s %s", actorType, actorID, label)
	}
	if e.countAudit("mcp.tools_list") != 1 {
		t.Fatal("audit tools_list")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var used sql.NullTime
		qq := "SELECT last_used_at FROM " + e.schema + ".api_keys WHERE id = $1"
		if err := e.admin.QueryRow(qq, e.keyID).Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used.Valid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("last_used_at tidak terisi")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestT07_AC2_UniformRejection(t *testing.T) {
	e := setupMCP(t)
	revokedKey, revokedID := e.newKey("to-revoke")
	if resp, b := e.rest(http.MethodPost, "/api/projects/"+e.projID+"/api-keys/"+revokedID+"/revoke", nil); resp.StatusCode/100 != 2 {
		t.Fatalf("revoke: %d %s", resp.StatusCode, b)
	}
	var bodies []string
	for _, k := range []string{"", "gr_zzzzzzzz_" + strings.Repeat("A", 43), revokedKey} {
		resp, b := e.mcp(k, pingBody, nil)
		if resp.StatusCode != 401 {
			t.Fatalf("key %q: %d %s", k, resp.StatusCode, b)
		}
		bodies = append(bodies, b)
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("body tidak seragam: %q", bodies)
	}
	if n := e.countAudit(rejAction); n != 3 {
		t.Fatalf("audit rejected = %d", n)
	}
}

func TestT07_AC5_RevokeTakesEffectNextRequest(t *testing.T) {
	e := setupMCP(t)
	if resp, b := e.mcp(e.rawKey, listBody, nil); resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if resp, b := e.rest(http.MethodPost, "/api/projects/"+e.projID+"/api-keys/"+e.keyID+"/revoke", nil); resp.StatusCode/100 != 2 {
		t.Fatalf("revoke: %d %s", resp.StatusCode, b)
	}
	if resp, _ := e.mcp(e.rawKey, listBody, nil); resp.StatusCode != 401 {
		t.Fatalf("setelah revoke: %d", resp.StatusCode)
	}
}

func TestT07_EndpointTransportOverRealServer(t *testing.T) {
	e := setupMCP(t)
	k := e.rawKey

	resp, b := e.mcp(k, pingBody, map[string]string{"Content-Type": "text/plain"})
	if resp.StatusCode != 415 {
		t.Fatalf("415: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.mcp(k, `{"jsonrpc":`, nil); resp.StatusCode != 400 || !strings.Contains(b, "-32700") {
		t.Fatalf("parse: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.mcp(k, `[`+pingBody+`]`, nil); resp.StatusCode != 400 || !strings.Contains(b, "-32600") {
		t.Fatalf("batch: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.mcp(k, pingBody, nil); resp.StatusCode != 200 || !strings.Contains(b, `"result":{}`) {
		t.Fatalf("ping: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.mcp(k, `{"jsonrpc":"2.0","id":4,"method":"resources/list"}`, nil); resp.StatusCode != 200 || !strings.Contains(b, "-32601") {
		t.Fatalf("-32601: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.mcp(k, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); resp.StatusCode != 202 || b != "" {
		t.Fatalf("202: %d %q", resp.StatusCode, b)
	}
	if resp, _ = e.mcp(k, pingBody, map[string]string{"Origin": "https://evil.example"}); resp.StatusCode != 403 {
		t.Fatalf("origin: %d", resp.StatusCode)
	}
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		req, _ := http.NewRequest(m, e.base+"/mcp", nil)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 405 || r.Header.Get("Allow") != "POST" {
			t.Fatalf("%s: %d", m, r.StatusCode)
		}
	}
	// endpoint hanya menulis initialize/tools_list: ping, -32601, notification, 4xx tidak beraudit.
	for _, a := range []string{"mcp.initialize", "mcp.tools_list", "mcp.unknown_tool"} {
		if n := e.countAudit(a); n != 0 {
			t.Fatalf("%s: %d entri tak terduga", a, n)
		}
	}
}

func newJar() http.CookieJar {
	j, _ := cookiejar.New(nil)
	return j
}
