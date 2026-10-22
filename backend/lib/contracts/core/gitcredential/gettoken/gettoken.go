// Package gettoken: kontrak core.gitcredential.get-token (T14).
// Token TIDAK BOLEH di-log, diaudit, di-Publish, disimpan, atau dikirim ke klien.
// Pemanggil WAJIB mengisi UserID dari identity request (bukan dari data tersimpan),
// sehingga PAT user lain tidak pernah dipakai.
package gettoken

import "grimoire/lib/common/secret"

type Request struct {
	UserID   string // uuid
	Provider string // "github" | "gitlab"
	Host     string // "github.com" | host gitlab ternormalisasi
}

type Response struct {
	Found      bool          // false = user belum menyimpan token untuk (provider, host)
	Unreadable bool          // Found && dekripsi gagal (kunci berganti) -> minta simpan ulang
	Disabled   bool          // GIT_CREDENTIAL_KEY kosong; Found/Token tidak bermakna
	Token      secret.String // hanya terisi bila Found && !Unreadable && !Disabled
}
