package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

var chgT0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func fileItem(path string, updated time.Time) listchanges.Item {
	return listchanges.Item{Path: path, SizeBytes: 10, CreatedAt: updated.Add(-time.Hour), UpdatedAt: updated}
}

func specItem(name string, updated time.Time) listspecs.Spec {
	return listspecs.Spec{Name: name, SizeBytes: 20, CreatedAt: updated.Add(-time.Hour), UpdatedAt: updated}
}

func names(p ChangePage) []string {
	out := []string{}
	for _, c := range p.Items {
		out = append(out, c.Kind+":"+c.Path+c.Name)
	}
	return out
}

func TestMergeChanges_OrderAndLimit(t *testing.T) {
	files := listchanges.Response{Items: []listchanges.Item{fileItem("f1.md", chgT0.Add(4*time.Hour)), fileItem("f2.md", chgT0.Add(1*time.Hour))}, Total: 2}
	specs := []listspecs.Spec{specItem("s1", chgT0.Add(3*time.Hour)), specItem("s2", chgT0.Add(2*time.Hour))}
	p := MergeChanges(chgT0, "all", files, specs, 3)
	if got := strings.Join(names(p), ","); got != "file:f1.md,spec:s1,spec:s2" || p.Total != 4 {
		t.Fatalf("%s total=%d", got, p.Total)
	}
}

func TestMergeChanges_StableTieBreak(t *testing.T) {
	ts := chgT0.Add(time.Hour)
	files := listchanges.Response{Items: []listchanges.Item{fileItem("B.md", ts), fileItem("a.md", ts)}, Total: 2}
	specs := []listspecs.Spec{specItem("a", ts)}
	p := MergeChanges(chgT0, "all", files, specs, 10)
	if got := strings.Join(names(p), ","); got != "file:B.md,file:a.md,spec:a" {
		t.Fatalf("%s", got)
	}
}

func TestMergeChanges_SpecsFilteredBySince(t *testing.T) {
	specs := []listspecs.Spec{specItem("old", chgT0.Add(-time.Hour)), specItem("new", chgT0.Add(time.Hour)), specItem("edge", chgT0)}
	p := MergeChanges(chgT0, "specs", listchanges.Response{}, specs, 10)
	if got := strings.Join(names(p), ","); got != "spec:new,spec:edge" || p.Total != 2 {
		t.Fatalf("%s total=%d", got, p.Total)
	}
}

func TestMergeChanges_CreatedFlag(t *testing.T) {
	files := listchanges.Response{Items: []listchanges.Item{
		{Path: "new.md", CreatedAt: chgT0.Add(time.Hour), UpdatedAt: chgT0.Add(time.Hour)},
		fileItem("upd.md", chgT0.Add(time.Minute)),
	}, Total: 2}
	p := MergeChanges(chgT0, "files", files, nil, 10)
	if !p.Items[0].Created || p.Items[1].Created {
		t.Fatalf("%+v", p.Items)
	}
	txt := p.Result().Text
	if !strings.Contains(txt, "- [file] new.md · dibuat · ") || !strings.Contains(txt, "- [file] upd.md · diperbarui · ") {
		t.Fatalf("text:\n%s", txt)
	}
}

func TestChangePage_ResultShape(t *testing.T) {
	files := listchanges.Response{Items: []listchanges.Item{fileItem("a.md", chgT0.Add(time.Hour))}, Total: 3}
	specs := []listspecs.Spec{specItem("pay", chgT0.Add(2*time.Hour))}
	p := MergeChanges(chgT0, "all", files, specs, 2)
	r := p.Result()
	if r.IsError || r.TargetType != "" || r.Target != "" {
		t.Fatalf("%+v", r)
	}
	if len(r.Summary) != 2 || r.Summary["count"] != 2 || r.Summary["total"] != 4 {
		t.Fatalf("summary = %v", r.Summary)
	}
	b, err := json.Marshal(r.Structured)
	if err != nil || strings.Contains(string(b), "updated_by") {
		t.Fatalf("structured = %s err=%v", b, err)
	}
	items := r.Structured["items"].([]map[string]any)
	if items[0]["kind"] != "spec" || items[0]["name"] != "pay" || items[0]["path"] != nil || items[1]["path"] != "a.md" || items[1]["name"] != nil {
		t.Fatalf("items = %v", items)
	}
	if r.Structured["truncated"] != true || r.Structured["since"] != "2026-10-20T12:00:00Z" {
		t.Fatalf("structured = %v", r.Structured)
	}
	for _, w := range []string{"2 perubahan sejak 2026-10-20T12:00:00Z (total 4):", "- [spec] pay · ", "[ada 2 perubahan lagi; persempit since/path/kind atau naikkan limit (maks 50)]"} {
		if !strings.Contains(r.Text, w) {
			t.Errorf("text tanpa %q:\n%s", w, r.Text)
		}
	}
}

func TestChangePage_Empty(t *testing.T) {
	p := MergeChanges(chgT0, "all", listchanges.Response{}, nil, 20)
	r := p.Result()
	if r.Text != "Tidak ada perubahan sejak 2026-10-20T12:00:00Z." || r.IsError || r.Summary["count"] != 0 {
		t.Fatalf("%+v", r)
	}
	if items := r.Structured["items"].([]map[string]any); items == nil || len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
}
