package base

import (
	"fmt"

	"grimoire/lib/common/errs"
)

// Kode error milik module ini; didaftarkan di registerCodes (gitsync/base.go).
const (
	CodeCredentialMissing    errs.Code = "CREDENTIAL_MISSING"    // 422
	CodeCredentialUnreadable errs.Code = "CREDENTIAL_UNREADABLE" // 422
	CodeInvalidRepoURL       errs.Code = "INVALID_REPO_URL"      // 400
	CodeGitTokenRejected     errs.Code = "GIT_TOKEN_REJECTED"    // 422
	CodeGitAccessDenied      errs.Code = "GIT_ACCESS_DENIED"     // 422
	CodeGitNotFound          errs.Code = "GIT_NOT_FOUND"         // 404
	CodeSyncLimitExceeded    errs.Code = "SYNC_LIMIT_EXCEEDED"   // 422
	CodeSyncInProgress       errs.Code = "SYNC_IN_PROGRESS"      // 409
	CodeGitTreeTooLarge      errs.Code = "GIT_TREE_TOO_LARGE"    // 422
)

const settingsPath = "/profile#token-git"

// Sentinel dipakai bersama; usecase TIDAK boleh memodifikasinya. Pesan tidak memuat token atau URL penuh.
var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrUnauthenticated = errs.New(errs.CodeUnauthorized, "perlu login")
	ErrInvalidKind     = errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
		map[string]any{"fields": map[string]string{"kind": "kind harus knowledge atau api_spec"}})
	ErrInvalidTargetPrefix = errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
		map[string]any{"fields": map[string]string{"target_prefix": "folder tujuan tidak valid"}})
	ErrFolderNotFound = errs.WithDetails(CodeGitNotFound, "folder tidak ditemukan di branch ini",
		map[string]any{"what": "folder"})

	ErrCredentialMissing    = errs.New(CodeCredentialMissing, "Anda belum menyimpan token Git")
	ErrCredentialUnreadable = errs.New(CodeCredentialUnreadable,
		"token Git Anda tidak dapat dibaca (kunci server berganti); simpan ulang token")
	ErrSyncInProgress = errs.New(CodeSyncInProgress, "sync lain untuk project dan jenis ini sedang berjalan")

	ErrGitTokenRejected = errs.New(CodeGitTokenRejected, "token Git ditolak provider; simpan ulang token")
	ErrGitAccessDenied  = errs.New(CodeGitAccessDenied, "token tidak punya akses ke repo ini")
	ErrGitRepoNotFound  = errs.WithDetails(CodeGitNotFound, "repo/branch tidak ditemukan atau token tidak punya akses",
		map[string]any{"what": "repo"})
	ErrGitTreeTooLarge = errs.New(CodeGitTreeTooLarge, "repo terlalu besar untuk dibaca sekaligus; persempit folder")

	ErrFeatureDisabled = errs.New(errs.CodeGitFeatureDisabled, "sync Git belum dikonfigurasi oleh admin")
	ErrUpstream        = errs.New(errs.CodeGitUpstream, "provider Git tidak dapat dihubungi, coba lagi")
	ErrTimeout         = errs.New(errs.CodeTimeout, "sync melebihi batas waktu")
)

// CredentialMissing -> 422 dengan details {provider, host, settings_path}.
func CredentialMissing(provider, host string) *errs.Error {
	return errs.WithDetails(CodeCredentialMissing, "Anda belum menyimpan token Git untuk "+host,
		map[string]any{"provider": provider, "host": host, "settings_path": settingsPath})
}

// CredentialUnreadable -> 422 dengan details {provider, host, settings_path}.
func CredentialUnreadable(provider, host string) *errs.Error {
	return errs.WithDetails(CodeCredentialUnreadable, ErrCredentialUnreadable.Message,
		map[string]any{"provider": provider, "host": host, "settings_path": settingsPath})
}

// InvalidRepoURL -> 400 INVALID_REPO_URL; field (opsional) menunjuk input yang salah.
func InvalidRepoURL(msg, field string) *errs.Error {
	var details any
	if field != "" {
		details = map[string]any{"field": field}
	}
	return errs.WithDetails(CodeInvalidRepoURL, msg, details)
}

// HostNotAllowed -> 422 HOST_NOT_ALLOWED dengan details {host}.
func HostNotAllowed(host string) *errs.Error {
	return errs.WithDetails(errs.CodeHostNotAllowed,
		"host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS",
		map[string]any{"host": host})
}

// SyncInProgress -> 409 dengan details {kind}.
func SyncInProgress(k Kind) *errs.Error {
	return errs.WithDetails(CodeSyncInProgress, ErrSyncInProgress.Message, map[string]any{"kind": string(k)})
}

// FolderNotFound -> 404 GIT_NOT_FOUND dengan details {what: "folder", folder}.
func FolderNotFound(folder string) *errs.Error {
	return errs.WithDetails(CodeGitNotFound, ErrFolderNotFound.Message,
		map[string]any{"what": "folder", "folder": folder})
}

// SyncLimitFiles -> 422 SYNC_LIMIT_EXCEEDED karena jumlah file.
func SyncLimitFiles(files int, l Limits, knownBytes int64) *errs.Error {
	return errs.WithDetails(CodeSyncLimitExceeded,
		fmt.Sprintf("folder berisi %d file, melebihi batas %d file per sync; persempit folder", files, l.MaxFiles),
		map[string]any{"limit": "files", "files": files, "max_files": l.MaxFiles,
			"known_bytes": knownBytes, "max_total_bytes": l.MaxTotalBytes})
}

// SyncLimitBytes -> 422 SYNC_LIMIT_EXCEEDED karena total ukuran.
func SyncLimitBytes(files int, l Limits, knownBytes int64) *errs.Error {
	return errs.WithDetails(CodeSyncLimitExceeded,
		fmt.Sprintf("total ukuran %d byte melebihi batas %d byte per sync; persempit folder", knownBytes, l.MaxTotalBytes),
		map[string]any{"limit": "bytes", "files": files, "max_files": l.MaxFiles,
			"known_bytes": knownBytes, "max_total_bytes": l.MaxTotalBytes})
}
