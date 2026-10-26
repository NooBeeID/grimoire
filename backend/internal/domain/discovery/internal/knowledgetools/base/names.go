// Package base memuat entity dan rule murni module knowledge-tools (tanpa I/O).
package base

const (
	ToolListFiles       = "list_files"
	ToolReadFile        = "read_file"
	ToolSearchKnowledge = "search_knowledge"

	// T18.
	ToolGetFileOutline    = "get_file_outline"
	ToolReadSection       = "read_section"
	ToolListRecentChanges = "list_recent_changes"
)

// ProjectArgAliases adalah nama argumen project yang ditolak dengan pesan khusus (AC5).
var ProjectArgAliases = []string{"project_id", "projectId", "project"}
