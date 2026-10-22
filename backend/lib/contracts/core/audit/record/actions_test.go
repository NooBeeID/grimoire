package record

import (
	"regexp"
	"slices"
	"testing"
)

var actionRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// catalog adalah salinan literal katalog di docs/tickets/README.md.
var catalog = []string{
	"mcp.list_files", "mcp.read_file", "mcp.search_knowledge", "mcp.list_api_specs",
	"mcp.list_api_endpoints", "mcp.get_api_endpoint", "mcp.auth.rejected", "mcp.rate_limited",
	"mcp.initialize", "mcp.tools_list", "mcp.unknown_tool",
	"auth.login.success", "auth.login.failed", "auth.logout", "auth.password.changed",
	"auth.password.reset", "user.created", "user.updated", "user.deactivated", "user.activated",
	"user.invited", "user.invitation.resent", "user.invitation.accepted", "user.invitation.email_failed",
	"project.created", "project.updated", "project.deleted", "project.member.added",
	"project.member.removed", "project.member.role_changed",
	"project.member.invited", "project.invitation.revoked",
	"apikey.created", "apikey.revoked",
	"knowledge.file.created", "knowledge.file.updated", "knowledge.file.deleted", "knowledge.file.moved",
	"knowledge.folder.deleted", "knowledge.upload.completed",
	"apispec.uploaded", "apispec.updated", "apispec.deleted",
	"git_credential.saved", "git_credential.deleted", "git_sync.run",
}

func TestActions_UniqueAndWellFormed(t *testing.T) {
	got := Actions()
	if len(got) != 46 {
		t.Fatalf("want 46 actions, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, a := range got {
		if seen[a] {
			t.Errorf("duplicate action %q", a)
		}
		seen[a] = true
		if !actionRe.MatchString(a) {
			t.Errorf("action %q does not match pattern", a)
		}
	}
}

func TestActions_MatchesCatalog(t *testing.T) {
	got := slices.Clone(Actions())
	want := slices.Clone(catalog)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("actions differ from catalog:\n got %v\nwant %v", got, want)
	}
}

func TestActions_ReturnsCopy(t *testing.T) {
	a := Actions()
	a[0] = "x"
	if Actions()[0] == "x" {
		t.Fatal("Actions must return a copy")
	}
}
