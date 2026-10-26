// Package listgroups adalah kontrak bus project.searchsynonym.list-groups (T20).
// Provider: project/searchsynonym (buslistgroups). Consumer: project/knowledge (pencarian).
//
// Tanpa cek akses: consumer sudah memverifikasi project (HTTP check-access / API key). Query provider
// selalu dibatasi ProjectID (rule 6.10). Error: tipe salah -> bus.ErrBadRequestType; ProjectID bukan UUID ->
// error yang membungkus errs.ErrInvalid.
package listgroups

type Request struct{ ProjectID string }

// Response berisi kelompok term huruf kecil, urut created_at lalu id; nil/kosong bila tidak ada.
type Response struct{ Groups [][]string }
