# T01 core/auth — Usecase `logout` (POST /api/auth/logout)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (foundation F1 harus sudah ada: migrasi `sessions`, `base.HashToken`, `base.Shared`, `base/audit`, `PasswordChangeGate` dengan `POST /api/auth/logout` di `PasswordGateExempt`). Konvensi: `docs/tickets/README.md` dan `rules.txt`. Overview menang bila ada konflik.

Catatan nama package: overview menyebut `httpx`. Di kode T00 yang sudah merge, simbol itu ada di dua package: `grimoire/internal/infra/middleware` (`RequireAuth`, `SessionCookieName`, `KeyByIP`) dan `grimoire/internal/infra/http/respond` (`WriteJSON`, `WriteError`). Plan ini memakai nama package nyata tersebut. Bila F1 memindahkannya, ikuti lokasi akhirnya; nama simbol tidak berubah.

## Metadata

| Field | Nilai |
|---|---|
| Endpoint | `/api/auth/logout` |
| Method | POST |
| Version | tanpa prefix versi di URL (`info.version: 0.1.0` di `api/modules/core/auth.yml`) |
| Scope | backend (frontend ditangani `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Feature | `logout` (gelombang B, U5) |
| Akses | `RequireAuth`; password gate exempt (tetap boleh saat `must_change_password=true`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00) |
| Bus | tidak menyediakan, tidak mengonsumsi (kecuali `core.audit.record` lewat `base/audit`) |
| Audit | `auth.logout`, status `ok` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/logout/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `logout.Init`), `backend/internal/domain/core/internal/auth/auth_integration_test.go` (baris AC8/AC9 dan test logout) |

## Summary Plan

User yang sedang login mengakhiri sesinya. Handler membaca cookie `grimoire_session`, usecase menghitung `HMAC-SHA256(SESSION_SECRET, token)` lewat `base.HashToken`, repository menjalankan satu `DELETE FROM sessions WHERE token_hash = $1`, lalu handler menimpa cookie dengan `Max-Age=0` dan menjawab 204. Audit `auth.logout` dikirim lewat `base/audit` SETELAH delete (satu statement, tidak butuh transaksi), fail-open.

Keputusan desain:
- Idempoten: jumlah baris terhapus 0 (sesi sudah dihapus `purgesessions`, logout paralel, atau cabut oleh admin di antara resolve dan delete) tetap 204 dan cookie tetap dihapus. Audit hanya dikirim bila `deleted > 0`, agar satu aksi nyata menghasilkan tepat satu entri (AC9) dan logout ganda tidak menggandakan log.
- Cookie tidak ada padahal identitas ada (tidak mungkin lewat jalur sesi normal, defensif untuk jalur autentikasi lain nanti): lewati delete dan audit, tetap hapus cookie, 204.
- Tanpa sesi sama sekali: `RequireAuth` menjawab 401 `UNAUTHORIZED` sebelum handler dijalankan. Ini perilaku middleware, bukan logika usecase.
- Kegagalan DB: 500 generik, cookie TIDAK dihapus (sesi masih hidup di server, klien boleh mencoba lagi).
- Logout hanya mencabut sesi saat ini; sesi perangkat lain milik user tidak disentuh.
- Atribut cookie penghapus harus sama dengan cookie yang dibuat `login` (Name, Path `/`, HttpOnly, SameSite Lax, Secure dari config) supaya peramban benar-benar menimpa.
- Usecase tidak dipecah: satu `Logout`. Tidak ada rule bisnis baru, jadi tidak ada method entity baru; hash token memakai `base.HashToken` dari foundation.
- Tidak ada rate limit khusus (aksi murah, sudah terlindungi sesi dan CSRF).

## Plan

Urutan wajib. Prasyarat F1 harus ada; bila belum, hentikan dan laporkan, jangan menambahkannya di luar allowedPaths.

1. **Cek OpenAPI.** Pastikan `api/modules/core/auth.yml` memuat `POST /api/auth/logout` (security `sessionCookie`, header `X-Requested-With`, respons 204 dengan header `Set-Cookie`, 401, 500). Bila belum ada, tambahkan (F1 langkah 11 seharusnya sudah), lalu `make api-bundle` dan `make api-check` (rule 11.6, sebelum kode handler).
2. **`logout/params.go`** (transport, tanpa I/O selain membaca request):

   ```go
   package logout

   type Params struct {
       Token string           // nilai mentah cookie; "" bila cookie tidak ada
       Meta  base.RequestMeta // IP dan UserAgent untuk audit
   }

   // parseParams membaca cookie middleware.SessionCookieName dan membangun RequestMeta
   // {IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}.
   // Tidak pernah gagal: cookie tidak ada menghasilkan Token "".
   func parseParams(r *http.Request) Params
   ```
   Tidak ada body yang dibaca; body apa pun diabaikan.
3. **`logout/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6):

   ```go
   type Repository interface {
       DeleteSessionByHash(ctx context.Context, tokenHash []byte) (deleted int64, err error)
   }

   type Usecase struct {
       repo    Repository
       audit   audit.Recorder
       session base.SessionConfig // hanya Secret yang dipakai
   }

   func NewUsecase(repo Repository, rec audit.Recorder, cfg base.SessionConfig) *Usecase

   // Logout:
   //  1. p.Token == "" -> return nil (tanpa DB, tanpa audit).
   //  2. h := base.HashToken(u.session.Secret, p.Token)
   //  3. n, err := u.repo.DeleteSessionByHash(ctx, h); err -> fmt.Errorf("logout: hapus sesi: %w", err)
   //  4. n == 0 -> return nil (idempoten, tanpa audit)
   //  5. u.audit.Record(ctx, audit.Event{Action: "auth.logout", TargetType: "user",
   //       Target: identity.UserID, Args: map[string]any{}, Status: "ok", Meta: p.Meta})
   //     identity dari reqctx.IdentityFrom(ctx); Actor nil agar auditor memakai identity
   //     (actor_label = email). Record tidak mengembalikan error (fail-open).
   //  6. return nil
   func (u *Usecase) Logout(ctx context.Context, p Params) error
   ```
4. **`logout/repository.go`** (satu-satunya yang menyentuh SQL):

   ```go
   type repository struct{ db db.Querier }
   func NewRepository(q db.Querier) Repository

   const deleteSessionSQL = `DELETE FROM sessions WHERE token_hash = $1`
   // ExecContext lalu RowsAffected(). Tanpa transaksi, tanpa join. Memanfaatkan unique index
   // sessions_token_hash_key. Error dibungkus fmt.Errorf("delete session: %w", err).
   ```
5. **`logout/handler.go`**:

   ```go
   type Handler struct {
       uc           *Usecase
       cookieSecure bool
   }
   func NewHandler(uc *Usecase, cookieSecure bool) *Handler

   // ServeHTTP: p := parseParams(r); err := h.uc.Logout(r.Context(), p).
   //  err != nil -> respond.WriteError(w, r, err) (500 generik; cookie tidak diubah; Set-Cookie tidak dikirim).
   //  sukses -> http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookieName, Value: "",
   //     Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cookieSecure})
   //     lalu w.WriteHeader(http.StatusNoContent).
   ```
   `MaxAge: -1` pada `http.Cookie` menghasilkan `Max-Age=0` di header (perilaku stdlib) dan sama dengan cara middleware `Session` menghapus cookie.
6. **`logout/init.go`**:

   ```go
   func Init(d app.Deps, s base.Shared) {
       repo := NewRepository(d.DB)
       uc := NewUsecase(repo, audit.New(d.Bus, d.Log), s.Session)
       h := NewHandler(uc, s.Session.CookieSecure)
       d.API.With(middleware.RequireAuth()).Post("/auth/logout", h.ServeHTTP)
   }
   ```
   Sesuaikan prefix path dengan cara `d.API` dimount T00 (hasil akhir wajib `/api/auth/logout`; `TestRoutesRegistered` memverifikasi). Tidak ada `RequireRole`, tidak ada rate limit.
7. **`auth/base.go`**: tambahkan satu baris `logout.Init(d, shared)` di `Register` (rule: satu baris per feature, minim konflik merge).
8. **Test** (bagian Test Scenario): unit usecase, integrasi repository, integrasi HTTP.
9. **Verifikasi**: `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, archtest `TestT00_AC4_RepoIsClean` (logout tidak meng-import feature lain, tidak ada SQL di luar `repository.go`), `make api-check`.

Kontrak bus: tidak ada yang baru. Konsumsi `core.audit.record` (mode Call, fail-open) lewat `base/audit`; shape request sesuai overview 5.5:
`record.Request{ActorType: "user", ActorID: <uid>, ActorLabel: <email>, Action: "auth.logout", TargetType: "user", Target: <uid>, Args: {}, Status: "ok", IP, UserAgent, TraceID}`.

## Migration

Tidak ada. Memakai tabel `sessions` dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (F1). Tidak ada perubahan skema atau indeks (`sessions_token_hash_key` sudah melayani delete).

## Changes

Baru:
- `backend/internal/domain/core/internal/auth/logout/init.go`
- `backend/internal/domain/core/internal/auth/logout/handler.go`
- `backend/internal/domain/core/internal/auth/logout/params.go`
- `backend/internal/domain/core/internal/auth/logout/usecase.go`
- `backend/internal/domain/core/internal/auth/logout/repository.go`
- `backend/internal/domain/core/internal/auth/logout/usecase_test.go`
- `backend/internal/domain/core/internal/auth/logout/repository_integration_test.go`
- `backend/internal/domain/core/internal/auth/logout/handler_test.go`

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `logout.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go`: tambah baris `auth.logout` di tabel AC8 dan AC9.
- `api/modules/core/auth.yml` dan hasil bundle `api/openapi.yml`: hanya bila operasi logout belum ada.

Tidak disentuh: `base/`, `base/store` (query logout hanya dipakai satu feature, jadi tinggal di `repository.go` sesuai overview F1 langkah 8), feature lain, infra.

## Test Scenario

Penamaan unit mengikuti nama perilaku; integrasi lintas feature `TestT01_ACn_...` (rule 13.4).

### Unit usecase (`usecase_test.go`, repo dan recorder palsu)
- `TestLogout_HashesTokenAndDeletes`: repo palsu menangkap argumen; harus sama dengan `base.HashToken(secret, token)`, BUKAN token mentah.
- `TestLogout_DeletedRecordsAuditOnce`: n=1 menghasilkan tepat satu `audit.Event` dengan `Action=="auth.logout"`, `Status=="ok"`, `Target==identity.UserID`, `Args` kosong.
- `TestLogout_NoRowsIsIdempotent`: n=0 mengembalikan nil dan 0 audit.
- `TestLogout_EmptyTokenSkipsDB`: Token "" mengembalikan nil, repo tidak dipanggil, 0 audit.
- `TestLogout_RepoErrorWrapped`: error repo dikembalikan terbungkus (`errors.Is` menemukan sumber), 0 audit.
- `TestLogout_AuditEventHasNoToken`: marshal `Event` tidak memuat token mentah maupun hash.

### Integrasi repository (`repository_integration_test.go`, `db.TestDB` + migrasi)
- `TestRepo_DeleteSessionByHash_DeletesOnlyThatSession`: user punya dua sesi; hapus satu menyisakan satu; return 1.
- `TestRepo_DeleteSessionByHash_UnknownHash`: return 0, tanpa error.
- `TestRepo_DeleteSessionByHash_DoesNotTouchUser`: baris `users` tetap ada.

### Integrasi HTTP (`handler_test.go` dan `auth_integration_test.go`; router lengkap, bus nyata, handler `core.audit.record` palsu bila T02 belum merge)
- `TestT01_Logout_ClearsCookieAndSession`: login, `POST /api/auth/logout` dengan `X-Requested-With`: 204, body kosong, `Set-Cookie` bernama `grimoire_session` bernilai kosong dengan `Max-Age=0`, `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` sesuai config; baris sessions hilang.
- `TestT01_Logout_OldCookieNoLongerWorks`: setelah logout, `GET /api/auth/me` dengan cookie lama: 401 `UNAUTHORIZED` (dan middleware menghapus cookie).
- `TestT01_Logout_OnlyCurrentSession`: login dua kali (dua cookie); logout dengan cookie A; cookie B masih 200 di `/api/auth/me`.
- `TestT01_Logout_AllowedWhenMustChangePassword`: user dengan flag ganti password: logout 204 (bukan 403 `PASSWORD_CHANGE_REQUIRED`).
- `TestT01_Logout_Anonymous401`: tanpa cookie: 401 `UNAUTHORIZED`; cookie berisi token tak dikenal: 401 dan tidak ada audit.
- `TestT01_Logout_RequiresCSRFHeader`: tanpa `X-Requested-With`: 403 `CSRF_REQUIRED`, sesi tidak terhapus.
- `TestT01_Logout_Idempotent`: hapus baris sesi lewat SQL setelah request melewati middleware tidak bisa disimulasikan di HTTP; gunakan unit `NoRowsIsIdempotent` plus: dua logout berurutan, yang kedua 401 (sesi sudah hilang) dan audit tetap tepat 1.
- `TestT01_AC9_Logout_OneAuditEntry`: tepat satu `record.Request` dengan `Action=="auth.logout"`, `ActorType=="user"`, `ActorLabel` = email, `TargetType=="user"`, `Status=="ok"`, IP terisi.
- `TestT01_AC8_Logout_NoSecretsInLogsOrAudit`: log buffer dan `record.Request` tidak memuat token cookie, hash token, atau `$argon2id$`.
- `TestAudit_UnregisteredDoesNotFailLogout`: tanpa handler audit, logout tetap 204 dan log berisi `audit record gagal`.
- `TestRoutesRegistered`: `POST /api/auth/logout` terdaftar (tidak ada GET).

## E2E Scenarios

Positif:
1. User login di `/login`, menekan Logout: diarahkan ke `/login`; cookie `grimoire_session` hilang dari penyimpanan peramban; `GET /api/auth/me` mengembalikan 401.
2. Tombol Back setelah logout tidak menampilkan halaman terproteksi (data diambil ulang, 401, redirect ke `/login`).
3. User dengan flag `must_change_password` di halaman `/change-password` menekan Logout: berhasil (204), kembali ke `/login`.
4. Login di dua peramban; logout di peramban A; peramban B tetap login.
5. Login ulang setelah logout berhasil dan menghasilkan cookie baru dengan nilai berbeda.
6. Admin membuka audit log: ada entri `auth.logout` dengan actor user tersebut.

Negatif:
1. `POST /api/auth/logout` tanpa cookie (fetch manual): 401 `UNAUTHORIZED`.
2. `POST /api/auth/logout` tanpa `X-Requested-With`: 403 `CSRF_REQUIRED`.
3. `GET /api/auth/logout`: 405 (atau 404 sesuai router), sesi tetap hidup.
4. Memutar ulang cookie lama setelah logout: 401; sesi tidak hidup lagi (token dicabut di server, bukan sekadar dihapus di klien).
5. Logout dua kali cepat dari dua tab: salah satu 204, yang lain 204 atau 401; tidak ada 500, audit tepat satu.
6. Database mati saat logout: 500 `INTERNAL`, cookie tidak dihapus, UI menampilkan pesan gagal dan tetap di halaman; coba lagi setelah pulih berhasil.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire`.

### POST /api/auth/logout

Request:
```
POST /api/auth/logout HTTP/1.1
Cookie: grimoire_session=k3J9xQ0v...43char
X-Requested-With: grimoire
```
Tanpa body.

204 (body kosong):
```
HTTP/1.1 204 No Content
Set-Cookie: grimoire_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax
```
Hasil sama bila sesi sudah hilang saat delete (idempoten). Di dev dengan `COOKIE_SECURE=false`, atribut `Secure` tidak ada.

401 (tanpa cookie, cookie tak dikenal, atau sesi sudah tidak valid; untuk cookie tak valid middleware juga menyertakan `Set-Cookie` Max-Age=0):
```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

403 (tanpa header CSRF):
```json
{
  "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib ada", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```
(teks `message` mengikuti implementasi CSRF T00; yang dijamin adalah `code`.)

500 (kegagalan DB; tanpa `Set-Cookie`, pesan generik tanpa detail SQL):
```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```
(teks `message` mengikuti peta error T00.)

### Audit (bukan respons; dikirim lewat bus)
```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "budi@tim.id",
    Action: "auth.logout", TargetType: "user", Target: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    Args: map[string]any{}, Status: "ok",
    IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33",
}
```
