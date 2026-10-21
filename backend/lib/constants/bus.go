package constants

// Provider: T01 core/auth. Consumer: infra/middleware (session auth).
const BUS_CORE_AUTH_RESOLVE_SESSION = "core.auth.resolve-session"

// Provider: T01 core/auth. Consumer: T03, T04, T05, T06.
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
// Provider: T05 (files), T06 (specs), T04 (keys aktif). Sebelum provider merge, Call -> ErrNotRegistered.
const BUS_PROJECT_KNOWLEDGE_COUNT_FILES = "project.knowledge.count-files"
const BUS_PROJECT_APISPEC_COUNT_SPECS = "project.apispec.count-specs"
const BUS_PROJECT_APIKEY_COUNT_KEYS = "project.apikey.count-keys"
