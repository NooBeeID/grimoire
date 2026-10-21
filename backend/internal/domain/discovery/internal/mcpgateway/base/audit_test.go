package base

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

var testP = Principal{KeyID: "k1", KeyName: "laptop", KeyPrefix: "gr_ab12", ProjectID: "p1", ProjectName: "Payments", IP: "1.2.3.4", UserAgent: "ua"}

func TestToolCallAudit(t *testing.T) {
	o := Outcome{
		Result: toolkit.Result{
			Text: "SECRET-CONTENT", TargetType: "file", Target: "a.md",
			Summary: map[string]any{"bytes": 999, "count": 3},
		},
		OriginalBytes: 20, Truncated: true, ErrorCode: "timeout",
	}
	r := ToolCallAudit(testP, "read_file", map[string]any{"path": "a.md"}, o, 1500*time.Millisecond)
	if r.Action != "mcp.read_file" || r.ActorType != record.ActorAPIKey || r.ActorID != "k1" || r.ActorLabel != "laptop (gr_ab12)" ||
		r.ProjectID != "p1" || r.ProjectLabel != "Payments" || r.TargetType != "file" || r.Target != "a.md" ||
		r.IP != "1.2.3.4" || r.UserAgent != "ua" || r.Status != record.StatusOK {
		t.Fatalf("%+v", r)
	}
	if r.Result["bytes"] != len("SECRET-CONTENT") || r.Result["original_bytes"] != 20 || r.Result["truncated"] != true ||
		r.Result["duration_ms"] != int64(1500) || r.Result["error_code"] != "timeout" || r.Result["count"] != 3 {
		t.Fatalf("%v", r.Result)
	}
	if strings.Contains(fmt.Sprintf("%v", r), "SECRET-CONTENT") {
		t.Fatal("Text bocor ke audit")
	}
	o.Result.IsError = true
	if ToolCallAudit(testP, "read_file", nil, o, 0).Status != record.StatusError {
		t.Fatal("status error")
	}
	if _, ok := ToolCallAudit(testP, "read_file", nil, Outcome{}, 0).Result["error_code"]; ok {
		t.Fatal("error_code tidak boleh ada")
	}
}

func TestUnknownToolAudit(t *testing.T) {
	r := UnknownToolAudit(testP, strings.Repeat("x", 300), time.Second)
	if r.Action != record.ActionMCPUnknownTool || r.Status != record.StatusError || len(r.Target) != 128 ||
		r.Result["error_code"] != "unknown_tool" || r.Result["duration_ms"] != int64(1000) {
		t.Fatalf("%+v", r)
	}
}

func TestInitializeAudit(t *testing.T) {
	ok := InitializeAudit(testP, "2025-06-18", "2025-06-18", "claude", "1.0", true, time.Millisecond)
	if ok.Action != record.ActionMCPInitialize || ok.Status != record.StatusOK || ok.Result["protocol_version"] != "2025-06-18" ||
		ok.Args["client_name"] != "claude" {
		t.Fatalf("%+v", ok)
	}
	bad := InitializeAudit(testP, "1999", "", "c", "v", false, 0)
	if bad.Status != record.StatusError || bad.Result["error_code"] != "unsupported_version" || bad.Args["protocol_version"] != "1999" {
		t.Fatalf("%+v", bad)
	}
}

func TestToolsListAudit(t *testing.T) {
	r := ToolsListAudit(testP, 3, 0)
	if r.Action != record.ActionMCPToolsList || r.Result["count"] != 3 || r.Status != record.StatusOK {
		t.Fatalf("%+v", r)
	}
}

func TestDeniedAudits(t *testing.T) {
	a := AuthRejectedAudit("9.9.9.9", "ua", "gr_ab12", "revoked")
	if a.Action != record.ActionMCPAuthRejected || a.ActorType != record.ActorAnonymous || a.ActorID != "" || a.ActorLabel != "gr_ab12" ||
		a.Status != record.StatusDenied || a.Target != "gr_ab12" || a.TargetType != "api_key" || a.Args["reason"] != "revoked" {
		t.Fatalf("%+v", a)
	}
	if n := AuthRejectedAudit("ip", "ua", "", "missing"); n.Target != "" || n.TargetType != "" {
		t.Fatalf("%+v", n)
	}
	k := RateLimitedKeyAudit(testP, 5)
	if k.Action != record.ActionMCPRateLimited || k.ActorID != "k1" || k.Target != "k1" || k.Args["scope"] != "key" || k.Status != record.StatusDenied {
		t.Fatalf("%+v", k)
	}
	i := RateLimitedIPAudit("9.9.9.9", "ua", 7)
	if i.ActorType != record.ActorAnonymous || i.Target != "9.9.9.9" || i.Args["scope"] != "auth_ip" || i.Args["retry_after_seconds"] != 7 {
		t.Fatalf("%+v", i)
	}
}
