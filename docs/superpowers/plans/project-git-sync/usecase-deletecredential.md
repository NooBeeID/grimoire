# T14 core/git-credential — Usecase `deletecredential` (`DELETE /api/me/git-credentials/{provider}?host=`)

Melengkapi `docs/superpowers/plans/project-git-sync/overview.md` (FOUNDATION). Bila bertentangan, overview menang. Prasyarat: langkah 0 foundation sudah ada (migrasi `git_credentials`, `gitcredential/base/*`, `base/audit`, aksi audit `git_credential.deleted`, kode error). Hanya satu usecase: `deletecredential`. Tidak menyentuh `savecredential`, `listcredentials`, `bustoken`, atau plan lain.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T14 |
| Scope | fullstack (backend + potongan FE usecase ini; sisanya di `frontend-plan.md`) |
| Domain | core |
| Module | git-credential (`gitcredential`) |
| Feature | `deletecredential` |
| Endpoint | `/api/me/git-credentials/{provider}?host=` |
| Method | DELETE (tanpa body, tidak di-decode) |
| Version | Tanpa prefix versi (`/api`); `api/modules/core/git-credential.yml` `info.version` 0.1.0 |
| Auth | `RequireAuth` + CSRF global (`X-Requested-With`). Identity dari `reqctx`; query selalu dibatasi `user_id` sendiri (D7) |
| Respons sukses | 204 tanpa body |
| Audit | `git_credential.deleted` (`record.ActionGitCredentialDeleted`), via Call, fail-open, Args `{provider, host}` saja (tanpa token, tanpa last4) |
| Bus dikonsumsi | `core.audit.record` (via `base/audit`) |
| Bus disediakan | tidak ada |
| Tabel | `git_credentials` (`DELETE ... RETURNING 1`); tanpa perubahan skema |
| Kriteria T14 | 2, 13 |
| Resource root | `backend/internal/domain/core/internal/gitcredential` |
| allowedPaths | `.../gitcredential/deletecredential/` saja (+ satu baris `Init` di `gitcredential/base.go` dan satu baris di test registrasi route modul) |

## Summary Plan

User menghapus PAT miliknya sendiri untuk satu `(provider, host)`. Setelah itu `core.gitcredential.get-token` mengembalikan `Found=false`, sehingga sync oleh user itu menghasilkan `CREDENTIAL_MISSING` (kriteria 2). Konfigurasi sync project (`git_sync_configs`) tidak disentuh; anggota lain dan config tersimpan tidak terpengaruh.

Keputusan penting:

- Urutan persis (jangan diubah): identity (`ActorFrom`, 401) -> `ParseProvider` (400) -> `NormalizeHost` (host query opsional: github selalu `github.com`; gitlab kosong -> `gitlab.com`) -> `repo.Delete(userID, provider, host)` -> tidak ada baris -> `base.ErrCredentialNotFound` (404 `NOT_FOUND`, "token tidak ditemukan") -> audit -> 204.
- **Tidak memeriksa `HostPolicy`.** Usecase tidak punya dependensi ke `gitprovider`. Token untuk host GitLab yang sudah dikeluarkan dari `GIT_SYNC_ALLOWED_HOSTS` tetap bisa dihapus. `NormalizeHost` hanya normalisasi format (policy dipanggil usecase lain), kecuali github dengan host selain `github.com`/`api.github.com`/kosong yang ditolak `HOST_NOT_ALLOWED` (aturan rule entity, bukan allowlist config).
- **Tidak bergantung pada `GIT_CREDENTIAL_KEY`.** Hapus tetap jalan saat fitur nonaktif (kunci kosong); tidak butuh `secretbox`. User boleh membersihkan PAT yatim walau admin menonaktifkan fitur.
- Isolasi pemilik di SQL: `WHERE user_id = $1 AND provider = $2 AND host = $3`. Baris user lain, host lain, dan provider lain tidak tersentuh. "Bukan milik saya" dan "tidak ada" menghasilkan respons identik (404) sehingga keberadaan token user lain tidak bocor (tidak ada jalur untuk menyebut user lain di request ini).
- Satu statement `DELETE ... RETURNING 1` sudah atomik; tidak perlu transaksi. Dua DELETE serentak pada baris yang sama: tepat satu 204 dan satu 404; audit tepat satu.
- Audit hanya setelah delete sukses, `fail-open` (kegagalan audit di-log tanpa data sensitif, respons tetap 204). Kegagalan 401/400/404/500 = 0 audit.
- Method DELETE tanpa body; handler tidak membaca body. Tidak perlu `BodyLimit` khusus.
- Tidak mengirim `Publish`/event. Cache FE disegarkan komposer (lihat FE).

## Plan

Urutan wajib, TDD: tulis tes dahulu (merah), baru implementasi.

**D0. Verifikasi prasyarat (tanpa edit).** Pastikan ada: `base.ParseProvider`, `base.NormalizeHost`, `base.ActorFrom`, `base.ErrCredentialNotFound`, `base.ErrInvalidProvider`, `base/audit.Recorder` dan `audit.New(bus, log)`, `record.ActionGitCredentialDeleted`, tabel `git_credentials` (migrasi core). Simbol yang hilang: hentikan dan laporkan; jangan membuatnya di luar allowedPaths. Cek signature `audit.Event` dan `Init` yang dipakai foundation (`base.go`) dan ikuti persis; plan ini memakai bentuk `Event{Actor, Action, Provider, Host, Meta}` dan `Init(d app.Deps, r chi.Router)`. Bila foundation memakai bentuk lain, sesuaikan nama field saja tanpa mengubah perilaku.

**D1. Tes dahulu** (bagian Test Scenario): `usecase_test.go`, `handler_test.go`, `repository_integration_test.go`. Pastikan gagal kompilasi/merah.

**D2. `deletecredential/params.go`**

```go
package deletecredential

// Input adalah masukan usecase. Tanpa body: semua dari path/query/context.
type Input struct {
	Provider string // path {provider}, mentah
	Host     string // query ?host=, mentah, boleh kosong
	Meta     base.RequestMeta
}
```

`base.RequestMeta{IP, UserAgent}` memakai `base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())` seperti modul lain. Bila `base.RequestMeta` belum ada di gitcredential, pakai tipe yang disediakan `base/audit` (jangan menambah tipe baru di luar allowedPaths).

**D3. `deletecredential/usecase.go`**

```go
type repository interface {
	// Delete: DELETE FROM git_credentials WHERE user_id=$1 AND provider=$2 AND host=$3 RETURNING 1.
	// deleted=false bila tidak ada baris. userID bukan uuid -> (false, nil) TANPA query.
	Delete(ctx context.Context, userID string, p base.Provider, host string) (deleted bool, err error)
}

type Usecase struct {
	repo  repository
	audit audit.Recorder
}

func NewUsecase(repo repository, a audit.Recorder) *Usecase
```

`Execute(ctx, in Input) error`, urutan persis:

1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `p, err := base.ParseProvider(in.Provider)`; error dikembalikan apa adanya (`ErrInvalidProvider`, 400).
3. `host, err := base.NormalizeHost(p, in.Host)`; error dikembalikan apa adanya (400 untuk format host gitlab tidak valid; `HOST_NOT_ALLOWED` untuk github dengan host asing). TIDAK memanggil policy.
4. `deleted, err := u.repo.Delete(ctx, actor.ID, p, host)`; error -> `fmt.Errorf("deletecredential: delete: %w", err)`; `!deleted` -> `base.ErrCredentialNotFound`.
5. `u.audit.Record(ctx, audit.Event{Actor: actor, Action: record.ActionGitCredentialDeleted, Provider: string(p), Host: host, Meta: in.Meta})`. Args yang dibangun recorder hanya `{provider, host}`. Return value/err audit diabaikan (fail-open; recorder yang me-log).
6. `return nil`.

Larangan: tidak ada import `gitprovider`, `secretbox`, `config`; tidak membaca `actor.Role`.

**D4. `deletecredential/repository.go`**

```go
const deleteSQL = `DELETE FROM git_credentials WHERE user_id = $1::uuid AND provider = $2 AND host = $3 RETURNING 1`

func (r *Repo) Delete(ctx context.Context, userID string, p base.Provider, host string) (bool, error) {
	if !store.ValidUUID(userID) { return false, nil }
	var one int
	err := r.db.QueryRowxContext(ctx, deleteSQL, userID, string(p), host).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) { return false, nil }
	if err != nil { return false, err }
	return true, nil
}
```

Satu-satunya tempat SQL. Tidak ada `LIKE`, tidak ada interpolasi string. `Repo` memegang `*db.DB`; `newRepo(d.DB)`.

**D5. `deletecredential/handler.go`**

```go
type usecase interface{ Execute(ctx context.Context, in Input) error }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	in := Input{
		Provider: chi.URLParam(r, "provider"),
		Host:     r.URL.Query().Get("host"),
		Meta:     base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
	}
	if err := h.uc.Execute(r.Context(), in); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Handler tidak membaca body dan tidak mencatat query ke log.

**D6. `deletecredential/init.go`**

```go
// Init memasang DELETE /{provider} pada route group /me/git-credentials (RequireAuth sudah di group).
func Init(d app.Deps, r chi.Router) {
	uc := NewUsecase(newRepo(d.DB), audit.New(d.Bus, d.Log))
	r.Delete("/{provider}", newHandler(uc).ServeHTTP)
}
```

Route group `d.API.With(middleware.RequireAuth()).Route("/me/git-credentials", ...)` dimiliki `gitcredential/base.go` (foundation). Di sana tambah satu baris `deletecredential.Init(d, r)` di dalam closure `Route`. Tidak ada `BodyLimit`/`ExtendTimeout`. CSRF dan timeout default dari `api.Use` global.

**D7. Tes registrasi route.** Tambah satu kasus di `routes_test.go` modul: `DELETE /api/me/git-credentials/github` tanpa sesi -> 401; tanpa header CSRF -> 403; `GET` pada path yang sama -> 405 (tidak ada GET `/{provider}`).

**D8. OpenAPI.** Foundation sudah membuat path di `api/modules/core/git-credential.yml`. Verifikasi path `DELETE /me/git-credentials/{provider}` dengan parameter query `host` (opsional), respons `204`, `400`, `401`, `404`, `422` (HOST_NOT_ALLOWED), `500`. Rapikan deskripsi dan contoh bila perlu (mis. "host opsional; github selalu github.com; gitlab default gitlab.com; tidak diperiksa terhadap allowlist"). Jika berubah: `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau.

**D9. Frontend** `frontend/src/domain/core/git-credential/deletecredential/` (lihat bagian FE di Changes).

**D10. Verifikasi**

- `go test ./backend/internal/domain/core/internal/gitcredential/deletecredential/...`
- `go test -tags integration ./backend/internal/domain/core/internal/gitcredential/deletecredential/...` (DB test + migrasi core).
- `go test ./backend/internal/archtest/...`, `go vet`, lint.
- `grep -rn "gitprovider\|secretbox\|Role" deletecredential/*.go` (non-test) kosong.
- FE: `npm run test`, `npm run depcruise`, `npm run test:arch`.

## Migration

Tidak ada di usecase ini. Tabel `git_credentials` dibuat foundation (`backend/migrations/core/<VERSION>_create_git_credentials.sql`, overview 7.1). Unique `(user_id, provider, host)` melayani `DELETE` lewat indeks prefix. Tes integrasi memakai `db.Up` sehingga memakai skema final.

## Changes

| File | Perubahan |
|---|---|
| `backend/internal/domain/core/internal/gitcredential/deletecredential/init.go` | baru: `Init(d, r)` |
| `.../deletecredential/handler.go` | baru: ambil path/query, panggil usecase, 204 |
| `.../deletecredential/params.go` | baru: `Input` |
| `.../deletecredential/usecase.go` | baru: `Usecase`, interface `repository`, orkestrasi |
| `.../deletecredential/repository.go` | baru: `DELETE ... RETURNING 1` |
| `.../deletecredential/usecase_test.go` | baru (fake repo + fake audit) |
| `.../deletecredential/handler_test.go` | baru |
| `.../deletecredential/repository_integration_test.go` | baru (tag `integration`) |
| `.../gitcredential/base.go` | ubah: satu baris `deletecredential.Init(d, r)` |
| `.../gitcredential/routes_test.go` (atau test registrasi modul) | ubah: satu kasus route DELETE |
| `api/modules/core/git-credential.yml`, `api/openapi.yml` | verifikasi/rapikan bila perlu |
| `frontend/src/domain/core/git-credential/deletecredential/index.ts` | baru: ekspor `DeleteCredentialDialog`, `deleteCredential`, tipe `DeleteTarget` |
| `.../deletecredential/types.ts` | baru: `DeleteTarget {provider: 'github'\|'gitlab'; host: string}` |
| `.../deletecredential/services/deletecredential.service.ts` (+ `.test.ts`) | baru: `deleteCredential(target)` |
| `.../deletecredential/views/DeleteCredentialDialog.svelte` (+ `.test.ts`) | baru: dialog konfirmasi |
| `frontend/src/shared/i18n/{id,en}.json` | ubah: kunci `gitCredential.delete.*` (digabung dengan kunci feature lain, tanpa mengubah kunci yang ada) |
| `.../gitcredential/{savecredential,listcredentials,bustoken}/`, `gitsync/`, store `gitCredentials` | TIDAK diubah oleh plan ini |

### Detail FE

- **Service** (`deletecredential.service.ts`), memakai `del` dari `shared/services/http` (CSRF otomatis, 204 -> `undefined`):

  ```ts
  export async function deleteCredential(t: DeleteTarget): Promise<void> {
    const url = `/api/me/git-credentials/${encodeURIComponent(t.provider)}`;
    await del<void>(url, { query: t.provider === 'gitlab' ? { host: t.host } : undefined });
  }
  ```
  GitHub tidak mengirim `host` (server selalu `github.com`). GitLab selalu mengirim host (termasuk `gitlab.com`).
- **View** `DeleteCredentialDialog.svelte`: props `open`, `target: DeleteTarget | null`, `onclose`, `ondeleted`. Dibangun di atas `shared/components/Dialog.svelte` + `Button` (pola `RevokeKeyDialog`/`DeleteFileDialog`; tidak ada komponen `ConfirmDialog` terpisah di `shared/components`, jadi dialog konfirmasi ini yang memenuhi peran itu). Judul: `gitCredential.delete.title` = "Hapus token Git". Isi: `gitCredential.delete.confirm` = "Hapus token {provider}? Sync yang memakai token ini akan berhenti bekerja untuk Anda." dengan `{provider}` = "GitHub" atau "GitLab (git.noobee.id)". Tombol: `common.cancel` (secondary) dan "Hapus" (`variant="danger"`, `loading` saat submit). Dialog tidak dapat ditutup saat submit (`dismissible={!submitting}`).
- **Alur sukses:** `await deleteCredential(target)` -> `onclose()` -> `ondeleted()` -> toast sukses `gitCredential.delete.success` ("Token {provider} dihapus"). `ondeleted` dipasang oleh komposer `GitTokensSection` (feature `listcredentials`), yang memanggil `gitCredentials.refresh()` (muat ulang store). Feature `deletecredential` TIDAK mengimpor store `gitCredentials` (feature tidak saling impor; depcruise hijau).
- **Error:** `404` -> tutup dialog, toast info `gitCredential.delete.gone` ("Token sudah tidak ada"), tetap panggil `ondeleted()` agar daftar segar. `400`/`422` dan lainnya -> tampilkan `e.message` + `traceId` di dalam dialog; dialog tetap terbuka. `401` ditangani `http.ts` (redirect login). Galat jaringan -> `String(e)`.
- **Visual (ringkas):** dialog `shadow-pop`, radius 16, lebar `max-w-sm`; pertanyaan "Token mana yang akan dihapus?" dijawab oleh baris ringkas di atas kalimat konfirmasi: ikon provider outline 16px + nama provider + host (mono) + `…7Qx2` (mono, `text-muted`). Teks konfirmasi `text-muted`, tombol utama `danger`. Tanpa elemen dekoratif tambahan. Data contoh: GitLab `git.noobee.id`, last4 `7Qx2`.

## Test Scenario

### Unit: `usecase_test.go` (fake repo, fake audit)

Fake repo merekam `userID, provider, host` dan jumlah panggilan; `deleted` dan `err` dapat diatur. Fake audit merekam `[]audit.Event`.

| Kasus | Setup | Harapan |
|---|---|---|
| Sukses github tanpa host | identity `u1`, provider `github`, host `""` | repo dipanggil `(u1, github, github.com)`; err nil; audit 1x: `Action=git_credential.deleted`, `Provider=github`, `Host=github.com` |
| github host `github.com` / `api.github.com` | host eksplisit | host ternormalisasi `github.com` |
| github host asing `evil.com` | | `HOST_NOT_ALLOWED`; repo 0 panggilan; audit 0 |
| Sukses gitlab host kosong | provider `gitlab`, host `""` | repo `(u1, gitlab, gitlab.com)` |
| Sukses gitlab self-hosted | host `HTTPS://Git.Noobee.ID/` | repo dipanggil dengan `git.noobee.id` |
| Tanpa policy | usecase dibangun tanpa dependensi policy/config; host gitlab `git.dikeluarkan.id` | sukses; membuktikan host di luar allowlist tetap bisa dihapus (kriteria: tidak memeriksa HostPolicy) |
| Provider tidak dikenal | `bitbucket`, `""` | `ErrInvalidProvider`; repo 0; audit 0 |
| Provider huruf besar | `GitHub` | diterima (dinormalisasi `github`) |
| Host gitlab tidak valid | `user@host`, `host/path` | 400; repo 0 |
| Tidak ada baris | `deleted=false` | `errors.Is(err, base.ErrCredentialNotFound)`; audit 0 |
| Tanpa login | ctx kosong | 401 `UNAUTHORIZED`; repo 0; audit 0 |
| Error repo | err DB | dibungkus `deletecredential: delete:`; audit 0; bukan `*errs.Error` (jadi 500 generik) |
| Audit gagal | fake audit error/panic-free | tetap `nil` (fail-open); delete sudah terjadi |
| Aktor dari identity | identity `u9` | repo menerima `u9` (bukan dari Input) |
| Args audit minimal | | event hanya memuat provider+host; marshal `Args` tidak memuat `token`, `last4`, `user_id` lain |
| Urutan | | `repo.Delete` sebelum `audit.Record` |

### Handler: `handler_test.go`

Memakai `Usecase` asli + fake repo in-memory yang memfilter per user + fake audit, dirutekan lewat chi; identity disuntik via `reqctx.WithIdentity`; `registerCodes()` dipanggil sekali (bukan `RegisterCodeStatus` langsung).

1. `DELETE /me/git-credentials/github` milik sendiri -> 204, body kosong.
2. `DELETE /me/git-credentials/gitlab?host=git.noobee.id` -> 204; fake repo menerima host `git.noobee.id`.
3. Ulang request yang sama -> 404, body JSON `error.code=NOT_FOUND`, `message="token tidak ditemukan"`.
4. Baris user B ada, A menghapus path yang sama -> 404 dan baris B tetap (respons identik dengan kasus "tidak ada": bandingkan byte setelah menghapus `trace_id`).
5. `DELETE /me/git-credentials/bitbucket` -> 400 `INVALID_ARGUMENT`.
6. `DELETE /me/git-credentials/github?host=evil.com` -> 422 `HOST_NOT_ALLOWED`.
7. Tanpa identity -> 401.
8. Body request diabaikan (kirim body JSON sembarang -> tetap 204).
9. Header respons 204 tidak memuat `Content-Type` JSON dan body 0 byte.

### Integration: `repository_integration_test.go` (tag `integration`, Postgres nyata)

Helper: buat dua user (A `0192f5a0-aaaa-7aaa-8bbb-00000000000a`, B `0192f5a0-bbbb-7aaa-8bbb-00000000000b`), `cred(t, user, provider, host)` memasukkan baris `git_credentials` dengan `token_enc` palsu 40 byte dan `last4 = '7Qx2'`.

1. `Delete_OwnRow`: A punya `github/github.com`; `Delete(A, github, github.com)` -> `true`; baris hilang; kedua kali -> `false`.
2. `Delete_OnlyOwn_OtherUserUntouched`: A dan B sama-sama punya `github/github.com`; A menghapus -> baris B tetap ada (COUNT = 1, `user_id = B`).
3. `Delete_OtherHost_DoesNotTouchOtherRows`: A punya `gitlab/gitlab.com` dan `gitlab/git.noobee.id`; hapus `git.noobee.id` -> `gitlab.com` tetap; hapus host yang tidak ada (`git.lain.id`) -> `false`, tidak ada baris berubah.
4. `Delete_OtherProvider`: A punya `github/github.com`; `Delete(A, gitlab, github.com)` -> `false` (tidak ada, constraint mencegah baris seperti itu); baris github tetap.
5. `Delete_UserMismatch_ReturnsFalse`: B menghapus `(github, github.com)` yang hanya dimiliki A -> `false`, baris A utuh.
6. `Delete_NonUUIDUser_NoQuery`: `userID = "bukan-uuid"` -> `(false, nil)` tanpa error SQL (cast uuid tidak dijalankan).
7. `Delete_Concurrent_ExactlyOne`: 8 goroutine `Delete` baris yang sama -> tepat satu `true`, sisanya `false`, tanpa error.
8. `Delete_DoesNotTouchSyncConfig`: buat project + `git_sync_configs` (kolom `last_synced_by = A`); hapus credential A -> baris config identik (kriteria 2: config tak disentuh).
9. `Execute_EndToEnd`: `Usecase` + `Repo` asli + fake audit: A menghapus -> nil, audit 1x; B (tanpa baris) -> `ErrCredentialNotFound`, audit 0, baris A tetap; host gitlab yang tidak ada di allowlist manapun (usecase tidak punya policy) tetap terhapus.
10. `AfterDelete_TokenGone`: setelah hapus, `SELECT count(*) FROM git_credentials WHERE user_id=A AND provider='github'` = 0 (basis kontrak `get-token` `Found=false`; tes bus ada di `bustoken`).

### Routes/arch

- `routes_test.go`: DELETE tanpa sesi -> 401, tanpa CSRF -> 403, ada sesi + CSRF -> mencapai handler.
- `go test ./backend/internal/archtest/...` hijau: `deletecredential` hanya meng-import `gitcredential/base/**`, `infra/*`, `lib/*`; tidak meng-import `savecredential`/`listcredentials`/`bustoken`, `gitprovider`, atau `secretbox`.

### FE unit

- `deletecredential.service.test.ts`: github -> `DELETE /api/me/git-credentials/github` tanpa `?host`; gitlab -> `?host=git.noobee.id`; header `X-Requested-With`; 204 -> resolve `undefined`; 404 -> `ApiError.status 404`; provider di-encode.
- `DeleteCredentialDialog.test.ts`: menampilkan teks "Hapus token GitHub? Sync yang memakai token ini akan berhenti bekerja untuk Anda."; teks GitLab memuat host; konfirmasi memanggil service lalu `onclose` + `ondeleted` + toast; submit mengunci tombol dan menonaktifkan penutupan; 404 -> toast "Token sudah tidak ada" + `ondeleted`; 422/500 -> pesan dan trace id di dialog, tidak menutup; Batal tidak memanggil service.
- Test arsitektur FE: `deletecredential` tidak mengimpor `listcredentials`/`savecredential`/`domain/project/**`.

## E2E Scenarios

Ditulis di `frontend/e2e/git-sync.spec.ts` (milik plan foundation/FE; bagian ini mendaftar skenario yang bergantung pada `deletecredential`). Setup: fake git server dan token sah `ghp_e2eValidToken7Qx2` (overview bagian 11).

Positif:
1. User login, `/profile#token-git`: baris GitHub "Tersimpan …7Qx2". Klik Hapus -> dialog "Hapus token GitHub? Sync yang memakai token ini akan berhenti bekerja untuk Anda." -> konfirmasi -> baris berubah menjadi "Belum ada" tanpa reload (store `gitCredentials` ter-refresh), toast sukses.
2. Setelah hapus, buka Workspace Knowledge > "Sync dari Git": kotak info "Atur Token Git", tombol Sync nonaktif; panggilan API `runSync` langsung -> 422 `CREDENTIAL_MISSING` (kriteria 2). Konfigurasi sync project tetap tampil (ringkasan terakhir tidak hilang).
3. Hapus token GitLab `git.noobee.id` (dua baris GitLab ada): hanya baris itu yang hilang; `gitlab.com` tetap.
4. Hapus token GitLab untuk host yang sudah dikeluarkan dari `GIT_SYNC_ALLOWED_HOSTS` (restart backend dengan allowlist kosong; baris lama masih tampil/ada di DB): `DELETE ...?host=git.lama.id` -> 204 (tidak diperiksa policy).
5. Simpan ulang token setelah hapus -> pil "Tersimpan" kembali dan sync berjalan normal.

Negatif:
1. Hapus token yang sudah dihapus (dua tab): tab kedua konfirmasi -> toast "Token sudah tidak ada", daftar ter-refresh; API langsung -> 404 `NOT_FOUND` "token tidak ditemukan".
2. User B menghapus `github` padahal hanya A yang punya token -> 404; token A masih ada dan sync A masih berhasil.
3. `DELETE /api/me/git-credentials/bitbucket` -> 400 `INVALID_ARGUMENT`.
4. `DELETE /api/me/git-credentials/github?host=evil.com` -> 422 `HOST_NOT_ALLOWED`.
5. Tanpa sesi -> 401; tanpa header CSRF -> 403.
6. Audit (halaman audit Super Admin): setelah hapus sukses ada event `git_credential.deleted` dengan Args `{provider, host}` saja; pencarian PAT uji `ghp_e2eValidToken7Qx2` dan `7Qx2` pada event itu tidak menemukan apa pun (kriteria 13). Hapus yang gagal (404) tidak menambah event.

## Request/Response body

Format error umum (`infra/http/respond`): `{"error": {"code", "message", "details"}, "trace_id"}`. Request: `DELETE /api/me/git-credentials/{provider}?host=...`, tanpa body, header `X-Requested-With: grimoire`, cookie sesi.

### Request

```
DELETE /api/me/git-credentials/gitlab?host=git.noobee.id HTTP/1.1
X-Requested-With: grimoire
Cookie: <sesi>
```

```
DELETE /api/me/git-credentials/github HTTP/1.1
X-Requested-With: grimoire
Cookie: <sesi>
```

### 204 sukses

Tanpa body.

### 404 token tidak ada (juga untuk token milik user lain)

```json
{ "error": { "code": "NOT_FOUND", "message": "token tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 400 provider tidak dikenal

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "provider": "provider harus github atau gitlab" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 400 host gitlab tidak valid (mis. `?host=user@host`)

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "host": "host tidak valid" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 422 github dengan host asing (`/github?host=evil.com`)

```json
{ "error": { "code": "HOST_NOT_ALLOWED", "message": "host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS", "details": { "host": "evil.com" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 500 galat DB (pesan generik, tanpa detail internal)

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### Audit (`record.Request` ringkas, setelah 204)

```json
{ "action": "git_credential.deleted", "actor_type": "user", "actor_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "target_type": "git_credential", "target": "gitlab:git.noobee.id",
  "args": { "provider": "gitlab", "host": "git.noobee.id" }, "status": "ok" }
```

Bus: tidak ada kontrak baru atau perubahan. Dikonsumsi: `core.audit.record` (Call, fail-open). Efek ke kontrak lain: `core.gitcredential.get-token` untuk `(userID, provider, host)` yang dihapus mengembalikan `Found=false` (diuji di `bustoken`).
