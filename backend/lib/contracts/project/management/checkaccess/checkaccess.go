// Package checkaccess adalah kontrak bus project.management.check-access (provider T03).
//
// Satu-satunya tempat keputusan akses project (rule 7.2). Module lain DILARANG menyalin logika keanggotaan.
//
// Pola consumer (T04-T06):
//
//	res, err := bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
//		checkaccess.Request{ProjectID: pid, UserID: id.UserID, UserRole: id.Role})
//	if err != nil { return err }                              // 500; fail-closed
//	if !res.ProjectExists || res.Level == checkaccess.LevelNone {
//		return errs.New(errs.CodeNotFound, "project tidak ditemukan") // 404, rule 7.3 (bukan 403)
//	}
//	// level cukup -> lanjut. Aksi yang butuh owner/admin: Level member -> 403 FORBIDDEN.
//
// Request dikirim sebagai nilai (bukan pointer).
package checkaccess

type Level string

const (
	LevelNone   Level = "none"
	LevelMember Level = "member"
	LevelOwner  Level = "owner"
	LevelAdmin  Level = "admin" // UserRole == super_admin
)

type Request struct {
	ProjectID string
	UserID    string
	UserRole  string // super_admin | developer
}

type Response struct {
	ProjectExists bool
	Level         Level
}
