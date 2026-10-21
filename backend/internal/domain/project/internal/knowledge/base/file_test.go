package base

import (
	"testing"
	"time"
)

func TestNewFileAndWithContent(t *testing.T) {
	a := Actor{ID: "u1", Role: "developer", Label: "Budi"}
	f := NewFile("p1", "docs/a.md", "# Judul\nisi", a)
	if f.Title != "Judul" || f.SizeBytes != len("# Judul\nisi") || len(f.Hash) != 32 || f.ProjectID != "p1" ||
		f.CreatedBy != "u1" || f.CreatedByLabel != "Budi" || f.UpdatedBy != "u1" {
		t.Fatalf("%+v", f)
	}
	b := Actor{ID: "u2", Label: "Sari"}
	g := f.WithContent("# Baru", b)
	if g.Title != "Baru" || g.UpdatedBy != "u2" || g.UpdatedByLabel != "Sari" || g.CreatedBy != "u1" || g.CreatedByLabel != "Budi" {
		t.Fatalf("%+v", g)
	}
	if g.Version() == f.Version() || f.Title != "Judul" {
		t.Fatal("WithContent must not mutate receiver and must change version")
	}
}

func TestToDTOAndConflict(t *testing.T) {
	now := time.Now().UTC()
	f := NewFile("p", "a.md", "# T", Actor{ID: "u", Label: "L"})
	f.CreatedAt, f.UpdatedAt = now, now
	d := f.ToDTO()
	if d.Version != Version(f.Hash) || d.CreatedBy.Label != "L" || d.UpdatedBy.ID != "u" || d.Path != "a.md" || d.Content != "# T" {
		t.Fatalf("%+v", d)
	}
	c := f.ToConflict()
	if c.Content != "# T" || c.Version != f.Version() || c.UpdatedByLabel != "L" || !c.UpdatedAt.Equal(now) {
		t.Fatalf("%+v", c)
	}
}
