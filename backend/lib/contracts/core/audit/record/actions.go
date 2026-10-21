package record

// Katalog aksi audit (lihat docs/tickets/README.md). Aksi baru: tambah konstanta
// di sini, di Actions(), dan di katalog README.
const (
	ActionMCPListFiles        = "mcp.list_files"
	ActionMCPReadFile         = "mcp.read_file"
	ActionMCPSearchKnowledge  = "mcp.search_knowledge"
	ActionMCPListAPISpecs     = "mcp.list_api_specs"
	ActionMCPListAPIEndpoints = "mcp.list_api_endpoints"
	ActionMCPGetAPIEndpoint   = "mcp.get_api_endpoint"
	ActionMCPAuthRejected     = "mcp.auth.rejected"
	ActionMCPRateLimited      = "mcp.rate_limited"
	ActionMCPInitialize       = "mcp.initialize"
	ActionMCPToolsList        = "mcp.tools_list"
	ActionMCPUnknownTool      = "mcp.unknown_tool"

	ActionAuthLoginSuccess    = "auth.login.success"
	ActionAuthLoginFailed     = "auth.login.failed"
	ActionAuthLogout          = "auth.logout"
	ActionAuthPasswordChanged = "auth.password.changed"
	ActionAuthPasswordReset   = "auth.password.reset"
	ActionUserCreated         = "user.created"
	ActionUserUpdated         = "user.updated"
	ActionUserDeactivated     = "user.deactivated"
	ActionUserActivated       = "user.activated"

	ActionUserInvited               = "user.invited"
	ActionUserInvitationResent      = "user.invitation.resent"
	ActionUserInvitationAccepted    = "user.invitation.accepted"
	ActionUserInvitationEmailFailed = "user.invitation.email_failed"

	ActionProjectCreated           = "project.created"
	ActionProjectUpdated           = "project.updated"
	ActionProjectDeleted           = "project.deleted"
	ActionProjectMemberAdded       = "project.member.added"
	ActionProjectMemberRemoved     = "project.member.removed"
	ActionProjectMemberRoleChanged = "project.member.role_changed"
	ActionProjectMemberInvited     = "project.member.invited"
	ActionProjectInvitationRevoked = "project.invitation.revoked"

	ActionAPIKeyCreated = "apikey.created"
	ActionAPIKeyRevoked = "apikey.revoked"

	ActionKnowledgeFileCreated     = "knowledge.file.created"
	ActionKnowledgeFileUpdated     = "knowledge.file.updated"
	ActionKnowledgeFileDeleted     = "knowledge.file.deleted"
	ActionKnowledgeFileMoved       = "knowledge.file.moved"
	ActionKnowledgeFolderDeleted   = "knowledge.folder.deleted"
	ActionKnowledgeUploadCompleted = "knowledge.upload.completed"
	ActionAPISpecUploaded          = "apispec.uploaded"
	ActionAPISpecUpdated           = "apispec.updated"
	ActionAPISpecDeleted           = "apispec.deleted"
)

// Actions mengembalikan salinan daftar katalog (dipakai tes dan sumber daftar UI).
func Actions() []string {
	return []string{
		ActionMCPListFiles, ActionMCPReadFile, ActionMCPSearchKnowledge,
		ActionMCPListAPISpecs, ActionMCPListAPIEndpoints, ActionMCPGetAPIEndpoint,
		ActionMCPAuthRejected, ActionMCPRateLimited,
		ActionMCPInitialize, ActionMCPToolsList, ActionMCPUnknownTool,
		ActionAuthLoginSuccess, ActionAuthLoginFailed, ActionAuthLogout,
		ActionAuthPasswordChanged, ActionAuthPasswordReset,
		ActionUserCreated, ActionUserUpdated, ActionUserDeactivated, ActionUserActivated,
		ActionUserInvited, ActionUserInvitationResent, ActionUserInvitationAccepted, ActionUserInvitationEmailFailed,
		ActionProjectCreated, ActionProjectUpdated, ActionProjectDeleted,
		ActionProjectMemberAdded, ActionProjectMemberRemoved, ActionProjectMemberRoleChanged,
		ActionProjectMemberInvited, ActionProjectInvitationRevoked,
		ActionAPIKeyCreated, ActionAPIKeyRevoked,
		ActionKnowledgeFileCreated, ActionKnowledgeFileUpdated, ActionKnowledgeFileDeleted,
		ActionKnowledgeFileMoved, ActionKnowledgeFolderDeleted, ActionKnowledgeUploadCompleted,
		ActionAPISpecUploaded, ActionAPISpecUpdated, ActionAPISpecDeleted,
	}
}
