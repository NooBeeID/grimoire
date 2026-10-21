// Package countmemberships adalah kontrak bus project.management.count-memberships
// (provider project/management, consumer core/auth purgeinvitations).
//
// Menghitung keanggotaan project per user. project_members.user_id tidak punya FK ke users,
// jadi auth bertanya lewat bus sebelum menghapus akun undangan yang kedaluwarsa.
//
// Pola consumer:
//
//	res, err := bus.CallTyped[countmemberships.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
//		countmemberships.Request{UserIDs: ids})
//	if err != nil { return err } // fail-closed: jangan hapus apa pun
//	if res.Counts[id] > 0 { /* masih punya keanggotaan */ }
//
// Provider WAJIB: hanya SELECT; id bukan uuid diabaikan; len(UserIDs) > MaxUserIDs -> error
// membungkus errs.ErrInvalid; satu query GROUP BY. Request dikirim sebagai nilai (bukan pointer).
package countmemberships

const MaxUserIDs = 500

type Request struct{ UserIDs []string }

// Counts: user tanpa keanggotaan absen (dibaca sebagai 0).
type Response struct{ Counts map[string]int }
