package constants

// Provider: T01 core/auth. Consumer: infra/middleware (session auth).
const BUS_CORE_AUTH_RESOLVE_SESSION = "core.auth.resolve-session"

// Provider: T01 core/auth. Consumer: T03, T04, T06 (T05 memakai reqctx.Identity, bukan get-user).
const BUS_CORE_AUTH_GET_USER = "core.auth.get-user"

// Provider: T01 core/auth. Consumer: T03.
const BUS_CORE_AUTH_FIND_DEVELOPERS = "core.auth.find-developers"

// Provider: T01 core/auth. Consumer: cmd/cli.
const BUS_CORE_AUTH_CREATE_ADMIN = "core.auth.create-admin"

// Provider: T02 core/auditlog. Consumer: T01, T03, T04, T05, T06, T07.
// Mode: Call untuk mcp.* dan aksi keamanan (rule 6.3), Publish untuk lainnya.
// Satu handler melayani kedua mode. Kegagalan = fail-open (lihat lib/contracts/core/audit/record).
const BUS_CORE_AUDIT_RECORD = "core.audit.record"

// Provider: T03 project/management. Consumer: T04, T05, T06 (check-access), T07 (get-project).
const BUS_PROJECT_MANAGEMENT_CHECK_ACCESS = "project.management.check-access"
const BUS_PROJECT_MANAGEMENT_GET_PROJECT = "project.management.get-project"

// Kontrak: lib/contracts/project/management/contentcount. Consumer: T03 (fail-soft).
// Provider: T05 (files, handler countfiles), T06 (specs), T04 (keys aktif). Sebelum provider merge, Call -> ErrNotRegistered.
const BUS_PROJECT_KNOWLEDGE_COUNT_FILES = "project.knowledge.count-files"
const BUS_PROJECT_APISPEC_COUNT_SPECS = "project.apispec.count-specs"
const BUS_PROJECT_APIKEY_COUNT_KEYS = "project.apikey.count-keys"

// Provider: T04 project/apikey. Consumer: T07 discovery/mcp-gateway.
// Kontrak: lib/contracts/project/apikey/verify (Call, fail-closed di consumer)
// dan lib/contracts/project/apikey/touch (Publish, fire-and-forget).
const BUS_PROJECT_APIKEY_VERIFY = "project.apikey.verify"
const BUS_PROJECT_APIKEY_TOUCH = "project.apikey.touch"

// Provider: T05 project/knowledge. Consumer: T08 (lewat gateway T07, ProjectID dari key terverifikasi).
// Kontrak: lib/contracts/project/knowledge/listfiles. Tanpa audit di provider (gateway yang mengaudit).
const BUS_PROJECT_KNOWLEDGE_LIST_FILES = "project.knowledge.list-files"

// Provider: T05. Consumer: T08. Kontrak: lib/contracts/project/knowledge/readfile.
// Offset/Limit dalam BYTE, dipotong pada batas karakter UTF-8. Tidak ketemu -> Found=false + Similar.
const BUS_PROJECT_KNOWLEDGE_READ_FILE = "project.knowledge.read-file"

// Provider: T18 project/knowledge (buslistchanges). Consumer: T18 discovery/knowledgetools (list_recent_changes).
// Kontrak: lib/contracts/project/knowledge/listchanges. Baca-saja, dibatasi ProjectID (rule 6.10).
const BUS_PROJECT_KNOWLEDGE_LIST_CHANGES = "project.knowledge.list-changes"

// Provider: T05. Consumer: T08. Kontrak: lib/contracts/project/knowledge/search. Snippet bersorot **term**.
const BUS_PROJECT_KNOWLEDGE_SEARCH = "project.knowledge.search"

// Provider: T06 project/apispec. Consumer: T09 (discovery/apispectools).
// Kontrak: lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}.
// Handler TIDAK memeriksa akses user; WAJIB membatasi query pada ProjectID request (rule 6.10).
const BUS_PROJECT_APISPEC_LIST_SPECS = "project.apispec.list-specs"
const BUS_PROJECT_APISPEC_LIST_ENDPOINTS = "project.apispec.list-endpoints"
const BUS_PROJECT_APISPEC_GET_ENDPOINT = "project.apispec.get-endpoint"

// Provider: T02 core/auditlog (feature scanentries, ditambah T11). Consumer: T11 (discovery/knowledgetools searchquality).
// Kontrak: lib/contracts/core/audit/scanentries. Baca-saja, selalu dibatasi ProjectID (rule 6.10).
const BUS_CORE_AUDIT_SCAN_ENTRIES = "core.audit.scan-entries"

// Provider: T11 discovery/knowledgetools (searchquality). Consumer: cmd/cli.
// Kontrak: lib/contracts/discovery/knowledgetools/searchquality. Baca-saja.
const BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT = "discovery.knowledgetools.search-quality-report"

// Provider: T12 core/auth (feature inviteuser). Consumer: project/management inviteuser.
// Kontrak: lib/contracts/core/auth/inviteuser. Deliver=false: hanya siapkan akun + undangan.
const BUS_CORE_AUTH_INVITE_USER = "core.auth.invite-user"

// Provider: T12 core/auth (feature resendinvitation). Consumer: project/management inviteuser, resendinvitation.
// Kontrak: lib/contracts/core/auth/resendinvitation. Menerbitkan password sementara baru dan mengirim email.
const BUS_CORE_AUTH_RESEND_INVITATION = "core.auth.resend-invitation"

// Provider: T12 project/management (feature countmemberships). Consumer: core/auth purgeinvitations.
// Kontrak: lib/contracts/project/management/countmemberships. Baca-saja.
const BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS = "project.management.count-memberships"

// Provider: T14 core/gitcredential (feature bustoken). Consumer: project/gitsync runsync, getconfig.
// Kontrak: lib/contracts/core/gitcredential/gettoken. Token tidak boleh di-log/diaudit/di-Publish/dikirim ke klien.
const BUS_CORE_GITCREDENTIAL_GET_TOKEN = "core.gitcredential.get-token"

// Provider: T14 project/knowledge (feature busupsertfiles). Consumer: project/gitsync runsync.
// Kontrak: lib/contracts/project/knowledge/upsertfiles. Menulis; tanpa cek keanggotaan (consumer wajib check-access).
const BUS_PROJECT_KNOWLEDGE_UPSERT_FILES = "project.knowledge.upsert-files"

// Provider: T14 project/apispec (feature busupsertspecs). Consumer: project/gitsync runsync.
// Kontrak: lib/contracts/project/apispec/upsertspecs. Menulis; tanpa cek keanggotaan (consumer wajib check-access).
const BUS_PROJECT_APISPEC_UPSERT_SPECS = "project.apispec.upsert-specs"
