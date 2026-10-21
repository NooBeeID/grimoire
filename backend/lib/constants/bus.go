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

// Provider: T05. Consumer: T08. Kontrak: lib/contracts/project/knowledge/search. Snippet bersorot **term**.
const BUS_PROJECT_KNOWLEDGE_SEARCH = "project.knowledge.search"

// Provider: T06 project/apispec. Consumer: T09 (discovery/apispectools).
// Kontrak: lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}.
// Handler TIDAK memeriksa akses user; WAJIB membatasi query pada ProjectID request (rule 6.10).
const BUS_PROJECT_APISPEC_LIST_SPECS = "project.apispec.list-specs"
const BUS_PROJECT_APISPEC_LIST_ENDPOINTS = "project.apispec.list-endpoints"
const BUS_PROJECT_APISPEC_GET_ENDPOINT = "project.apispec.get-endpoint"
