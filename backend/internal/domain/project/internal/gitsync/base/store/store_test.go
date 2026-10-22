package store

import (
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
)

func TestScanConfig(t *testing.T) {
	js := `{"status":"partial","summary":{"created":1,"updated":0,"unchanged":0,"skipped":0,"rejected":1},"items":[{"repo_path":"a","target":"a","status":"created","reason_code":"","reason":"","issues":[]}],"truncated":false,"error_code":"","error_message":""}`
	st, uid, lbl := "partial", "u1", "Rina"
	now := time.Now()
	c := ScanConfig(Row{ID: "i", ProjectID: "p", Kind: "knowledge", Provider: "github", Host: "github.com", Branch: "main",
		LastStatus: &st, LastSyncedAt: &now, LastSyncedBy: &uid, LastSyncedByLabel: &lbl, LastResult: &js})
	if c.Kind != base.KindKnowledge || c.LastResult == nil || c.LastResult.Summary.Rejected != 1 || len(c.LastResult.Items) != 1 {
		t.Fatalf("%+v", c)
	}
	bad := "{rusak"
	c = ScanConfig(Row{Kind: "api_spec", LastResult: &bad})
	if c.LastResult != nil {
		t.Fatal("jsonb rusak -> nil")
	}
	if ScanConfig(Row{}).LastResult != nil {
		t.Fatal("null -> nil")
	}
}

func TestMarshalResultCaps(t *testing.T) {
	var items []base.ReportItem
	for i := 0; i < base.MaxStoredReportItems+5; i++ {
		items = append(items, base.ReportItem{RepoPath: "f", Status: base.ItemCreated})
	}
	b, err := MarshalResult(base.NewReport(items))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"truncated":true`) || !strings.Contains(s, `"created":505`) || strings.Count(s, `"repo_path"`) != base.MaxStoredReportItems {
		t.Fatal(len(s))
	}
}

func TestValidUUID(t *testing.T) {
	if !ValidUUID("0192f5a0-aaaa-7aaa-8bbb-00000000000a") || ValidUUID("x") {
		t.Fatal()
	}
}
