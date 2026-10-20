# T01 core/auth: Usecase `resolvesession` (bus `core.auth.resolve-session`)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1 foundation: `base.Session`, `base.HashToken`, `base.Shared`, `base/store`, config sesi, migrasi `sessions`/`users`). Bila overview bertentangan dengan dokumen ini, overview menang. Hanya satu usecase: `resolvesession`.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | backend saja (tidak ada FE; konsumennya middleware `Session` T00) |
| Domain | core |
| Module | auth |
| Feature | `resolvesession` (gelombang A, U1) |
| Endpoint | bus `core.auth.resolve-session` (konstanta `constants.BUS_CORE_AUTH_RESOLVE_SESSION`, sudah ada dari T00) |
| Method | bus `Call` (bukan HTTP) |
| Version | kontrak T00 apa adanya (`lib/contracts/core/auth/resolvesession`), tidak diubah |
| Consumer | `middleware.Session` (T00), dipanggil pada setiap request ber-cookie `grimoire_session` |
| Auth | internal; tidak ada route HTTP |
| Audit | tidak ada (jalur panas, tidak tercatat di katalog audit) |
| Tabel / migrasi | membaca `sessions` JOIN `users`, menulis `sessions.last_seen_at`. Tidak ada migrasi baru |
| AC | AC5 (bersama `setuserstatus`) |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/resolvesession/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `resolvesession.Init`) |

Catatan nama package (kode T00 yang sudah merge): folder middleware ada di `backend/internal/infra/middleware` (konstanta `SessionCookieName`), router di `internal/infra/http`, scheduler di `internal/infra/jobs`. Plan ini hanya memakai simbol bus (`bus.Expect`, `bus.Handler`, `Registry.Register`), `db.Querier`, `app.Deps`.

Prasyarat dari F1 (jangan dikerjakan di sini; bila belum ada, hentikan dan laporkan lewat needsHumanInput): `base.Session` (`IsValid`, `NeedsTouch`), `base.User` (`CanLogin`), `base.HashToken`, `base.TokenBytes`, `base.TouchInterval`, `base.Shared` (`Session`, `Clock`), migrasi `core/20261021100000000_create_auth_tables.sql`, konfigurasi `SESSION_IDLE_TIMEOUT`/`SESSION_ABSOLUTE_TIMEOUT`.

## Summary Plan

`resolvesession` menerjemahkan token mentah cookie menjadi identitas terbaru. Alur:

1. Token dihitung `HashToken(SESSION_SECRET, token)` (HMAC-SHA256), lalu dicari di `sessions` (JOIN `users`) lewat `token_hash`. Token mentah tidak pernah disimpan atau di-log.
2. `Found=false` (tanpa error, rule 6.7) bila: token kosong atau panjang bukan 43 karakter (tanpa query DB), sesi tidak ada, `now >= expires_at`, `now >= last_seen_at + SESSION_IDLE_TIMEOUT`, atau user nonaktif.
3. Bila valid, `Response` diisi dari baris `users` saat ini (role, nama, email, `must_change_password` dibaca tiap request; perubahan role/flag berlaku di request berikutnya tanpa login ulang).
4. `last_seen_at` disentuh paling sering sekali per menit: `NeedsTouch(now)` di entity, dan SQL `UPDATE ... WHERE id=$1 AND last_seen_at < $2 - interval '1 minute'` supaya request paralel idempoten dan tidak menulis berlebihan.
5. Request bus salah tipe: `bus.Expect[resolvesession.Request]` mengembalikan error yang membungkus `bus.ErrBadRequestType` (rule 6.9). Error juga dikembalikan untuk kegagalan infra (DB). Middleware T00 memetakannya ke 500.

Keputusan:
- Aturan sesi (valid/idle/absolut/touch) ada di entity `base.Session`; `CanLogin` di `base.User`. Usecase hanya orkestrasi: hash, ambil, tanya entity, sentuh, petakan respons (rule 5.1, 5.2).
- Kegagalan menyentuh `last_seen_at` TIDAK menggagalkan request: dicatat `WARN` ("touch session gagal") dan hasil tetap `Found=true`. Alasan: pembacaan identitas sudah benar, touch hanya optimasi idle; gagal-tertutup di sini membuat gangguan DB sesaat memutus semua user. Konsekuensi: sesi bisa idle-expire lebih cepat bila DB terus gagal menulis; diterima.
- Sesi kedaluwarsa tidak dihapus di jalur request (tugas `purgesessions`), jadi jalur ini read-mostly.
- Satu query JOIN (bukan dua) untuk menekan latensi jalur panas. Tidak memakai transaksi: satu SELECT lalu satu UPDATE kondisional independen.
- Tidak memakai `base/audit` dan tidak memakai `base.Hasher`.
- Idle check memakai `last_seen_at` yang dibaca, bukan nilai setelah touch. Touch hanya terjadi bila sesi sudah dinyatakan valid.
- Usecase tidak dipecah artifisial: satu `Execute(ctx, req) (Response, error)`.

## Plan

Urutan wajib. Test ditulis bersama kode (TDD per langkah).

### P0. Verifikasi prasyarat (tanpa edit)
Pastikan simbol F1 di atas ada dan migrasi sudah menyediakan `sessions` dengan indeks unik `sessions_token_hash_key`. Pastikan `go build ./...` hijau.

### P1. `resolvesession/repository.go` (satu-satunya yang menyentuh SQL, rule 5.3)

```go
package resolvesession

// Repository dipakai usecase lewat interface (rule 5.6); implementasi konkret Repo.
type Repository interface {
    // Found=false bila tidak ada baris. Mengembalikan entity, bukan row SQL.
    FindSession(ctx context.Context, tokenHash []byte) (base.Session, base.User, bool, error)
    // Mengembalikan jumlah baris yang berubah (0 bila sudah disentuh request lain).
    TouchSession(ctx context.Context, sessionID string, now time.Time) (bool, error)
}

type Repo struct{ q db.Querier }
func NewRepo(q db.Querier) *Repo
```

SQL `FindSession`:

```sql
SELECT s.id, s.user_id, s.token_hash, s.created_at, s.last_seen_at, s.expires_at,
       u.email, u.name, u.role, u.is_active, u.must_change_password
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
```

- Scan ke struct row privat bertag `db`, lalu dipetakan ke `base.Session` dan `base.User` (`ID = user_id`, field lain dari `u.*`; `PasswordHash` TIDAK dipilih dan tidak dibaca). `sql.ErrNoRows` -> `found=false, err=nil`.
- Kolom `ip`/`user_agent` tidak dibaca (tidak perlu).

SQL `TouchSession`:

```sql
UPDATE sessions SET last_seen_at = $2
WHERE id = $1 AND last_seen_at < $2 - interval '1 minute'
```

`interval '1 minute'` konstan sengaja mencerminkan `base.TouchInterval` (komentar di SQL menunjuk konstanta itu; test AC memastikan konsistensi). Batas waktu `now` dikirim dari Clock Go (testable), bukan `now()` DB.

### P2. `resolvesession/usecase.go` (orkestrasi)

```go
type Usecase struct {
    repo   Repository
    clock  base.Clock
    cfg    base.SessionConfig // Secret, IdleTimeout
    log    *slog.Logger
}
func NewUsecase(repo Repository, clock base.Clock, cfg base.SessionConfig, log *slog.Logger) *Usecase

func (u *Usecase) Execute(ctx context.Context, req resolvesession.Request) (resolvesession.Response, error) {
    // 1. token kosong atau len != base64.RawURLEncoding.EncodedLen(base.TokenBytes) (43)
    //    -> Response{Found:false}, nil   (tanpa query)
    // 2. hash := base.HashToken(u.cfg.Secret, req.Token)
    // 3. sess, user, found, err := u.repo.FindSession(ctx, hash)
    //    err -> return {}, fmt.Errorf("resolvesession: cari sesi: %w", err)
    //    !found -> {Found:false}
    // 4. now := u.clock()
    //    if !sess.IsValid(now, u.cfg.IdleTimeout) || !user.CanLogin() -> {Found:false}
    // 5. if sess.NeedsTouch(now) {
    //        if _, err := u.repo.TouchSession(ctx, sess.ID, now); err != nil {
    //            u.log.WarnContext(ctx, "touch session gagal", "error", err)   // tanpa token/hash
    //        }
    //    }
    // 6. return Response{Found:true, UserID: user.ID, Email: user.Email, Name: user.Name,
    //                    Role: user.Role, MustChangePassword: user.MustChangePassword}, nil
}
```

Tanpa rule bisnis inline: kondisi valid/idle/touch seluruhnya dipanggil dari entity. `ctx` dipakai apa adanya (nilai `reqctx` dibawa bus).

### P3. `resolvesession/handler.go` (adapter bus, rule 6.9)

```go
func Handle(uc *Usecase) bus.Handler {
    return func(ctx context.Context, req any) (any, error) {
        r, err := bus.Expect[resolvesession.Request](req)
        if err != nil { return nil, err }       // membungkus bus.ErrBadRequestType
        return uc.Execute(ctx, r)               // mengembalikan resolvesession.Response (nilai, bukan pointer)
    }
}
```

Tipe kembalian harus `resolvesession.Response` bernilai (bukan pointer) agar `bus.CallTyped[resolvesession.Response]` T00 berhasil. Tidak ada `params.go` HTTP.

### P4. `resolvesession/init.go`

```go
func Init(d app.Deps, s base.Shared) {
    uc := NewUsecase(NewRepo(d.DB), s.Clock, s.Session, d.Log)
    d.Bus.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, Handle(uc))
}
```

(`d.DB` memenuhi `db.Querier`; bila T00 mengekspos Querier lewat field lain, sesuaikan, jangan ubah `Deps`.)

### P5. `auth/base.go`
Tambah satu baris `resolvesession.Init(d, shared)` di `Register`. Jangan menyentuh baris feature lain.

### P6. Verifikasi
`go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auth/resolvesession/...`, `go test ./internal/archtest/...` (feature tidak meng-import feature lain; hanya `base`, `base/store` opsional). Jalankan juga test integrasi middleware T00 dengan bus nyata bila ada.

## Migration

Tidak ada. Memakai tabel `sessions` dan `users` dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (F1). Indeks yang diandalkan: `sessions_token_hash_key` (unik) untuk lookup, PK `users.id` untuk join.

## Changes

Baru:
- `backend/internal/domain/core/internal/auth/resolvesession/init.go`
- `backend/internal/domain/core/internal/auth/resolvesession/handler.go`
- `backend/internal/domain/core/internal/auth/resolvesession/usecase.go`
- `backend/internal/domain/core/internal/auth/resolvesession/repository.go`
- `backend/internal/domain/core/internal/auth/resolvesession/usecase_test.go` (unit, repo palsu)
- `backend/internal/domain/core/internal/auth/resolvesession/handler_test.go` (bus nyata)
- `backend/internal/domain/core/internal/auth/resolvesession/repository_integration_test.go` (Postgres nyata)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `resolvesession.Init(d, shared)`.

Tidak diubah: kontrak `lib/contracts/core/auth/resolvesession`, konstanta bus, middleware T00, `base/*` (milik F1).

## Test Scenario

Penamaan rule 13.4. Unit memakai repo palsu in-memory dan clock terkontrol; integrasi memakai `db.TestDB(t)` + migrasi.

### Unit usecase (`usecase_test.go`, tabel)
- `TestResolveSession_ValidSession`: sesi valid, user aktif -> `Found=true` dengan UserID/Email/Name/Role/MustChangePassword sesuai baris user.
- `TestResolveSession_EmptyToken` dan `_WrongLength` (42 dan 44 karakter): `Found=false`, repo TIDAK dipanggil.
- `TestResolveSession_NotFound`: repo `found=false` -> `Found=false`, err nil.
- `TestResolveSession_Expired`: `now == expires_at` dan `now > expires_at` -> `Found=false` (batas eksklusif).
- `TestResolveSession_IdleTimeout`: `last_seen_at = now - IdleTimeout` -> `Found=false`; `now - IdleTimeout + 1s` -> `Found=true`.
- `TestResolveSession_InactiveUser`: sesi valid tetapi `IsActive=false` -> `Found=false`, tanpa touch.
- `TestResolveSession_HashesToken`: repo palsu menerima `HashToken(secret, token)`, bukan token mentah; secret berbeda -> hash berbeda.
- `TestResolveSession_TouchThrottle`: `last_seen_at` 59 detik lalu -> `TouchSession` tidak dipanggil; 60 detik -> dipanggil tepat sekali dengan `now`.
- `TestResolveSession_NoTouchWhenInvalid`: sesi idle/expired/user nonaktif -> tidak ada touch.
- `TestResolveSession_TouchErrorIsNonFatal`: `TouchSession` error -> `Found=true`, err nil, log WARN memuat "touch session gagal" dan tidak memuat token/hash.
- `TestResolveSession_FindErrorPropagates`: repo error -> `Found=false`, err dibungkus (`errors.Is` ke error asli), tanpa isi token di pesan.
- `TestResolveSession_FreshRoleAndFlag`: dua pemanggilan berurutan, repo mengubah role dan `must_change_password` di antaranya -> respons kedua mencerminkan nilai baru.

### Bus (`handler_test.go`, `bus.NewRegistry` nyata)
- `TestResolveSession_BadRequestType`: `reg.Call(ctx, endpoint, "string-salah")` dan `resolvesession.Request` bentuk pointer -> `errors.Is(err, bus.ErrBadRequestType)` (rule 6.9), repo tidak dipanggil.
- `TestResolveSession_CallTypedRoundTrip`: `bus.CallTyped[resolvesession.Response]` berhasil (memastikan handler mengembalikan nilai, bukan pointer).
- `TestResolveSession_EndpointRegistered`: setelah `Init`, endpoint `core.auth.resolve-session` terdaftar (bukan `ErrNotRegistered`).

### Integrasi repository (Postgres nyata)
- `TestResolveSessionRepo_FindJoinsUser`: user + sesi dimasukkan, `FindSession` mengembalikan field user benar dan tidak memuat `PasswordHash`.
- `TestResolveSessionRepo_UnknownHash`: hash acak -> `found=false`, tanpa error.
- `TestResolveSessionRepo_TouchConditional`: touch pada `last_seen_at` lama -> `true` dan kolom berubah; touch kedua dengan `now` +30 detik -> `false`, kolom tidak berubah (idempoten, 1 menit).
- `TestResolveSessionRepo_TouchConcurrent`: 10 goroutine menyentuh sesi sama, hasil akhir `last_seen_at` konsisten dan tidak ada error.
- `TestResolveSessionRepo_CascadeOnUserDelete`: menghapus user menghapus sesi (FK) sehingga `FindSession` `found=false`.

### Integrasi lintas fitur (dimiliki bersama; ditulis di `auth_integration_test.go` W, tabel diisi oleh usecase ini)
- `TestT01_AC5_DeactivateKillsSessionImmediately` (bagian resolvesession): user aktif punya sesi; user di-set `is_active=false` langsung di DB; request ber-cookie lama lewat router lengkap -> 401 dengan `Set-Cookie` `Max-Age=0`. Tanpa menunggu, tanpa cache.
- `TestSession_MiddlewareWithRealResolver`: `middleware.Session` + handler asli: cookie valid -> `reqctx.Identity` terisi; cookie tidak dikenal -> anonim + hapus cookie; cookie valid tetapi `must_change_password=true` -> `Identity.MustChangePassword=true` (penegakan oleh gate, bukan di sini).

## E2E Scenarios

Usecase ini tidak punya UI; skenario diverifikasi lewat perilaku end-to-end sesi (Playwright/HTTP terhadap binary, bergantung pada `login`).

Positif:
1. Login lalu `GET /api/auth/me` dengan cookie: 200 dengan identitas dari `resolve-session`.
2. Admin mengubah role developer menjadi super_admin; developer (sesi aktif) memanggil `GET /api/auth/me` lagi: role baru terlihat tanpa login ulang.
3. Aktivitas berkala (lebih dari 1 menit antar request) menjaga sesi hidup melewati `SESSION_IDLE_TIMEOUT` total; `sessions.last_seen_at` maju paling sering sekali per menit (periksa DB setelah 5 request berturut-turut dalam 10 detik: hanya satu kenaikan).
4. Admin mereset flag lewat `changepassword`: request berikutnya melihat `must_change_password=false`.

Negatif:
1. Cookie diedit satu karakter: 401, cookie dihapus, tidak ada 500.
2. Admin menonaktifkan user lain yang sedang login: request user itu berikutnya 401 (AC5), tanpa menunggu timeout.
3. Sesi tanpa aktivitas lebih lama dari `SESSION_IDLE_TIMEOUT` (atau lewat `SESSION_ABSOLUTE_TIMEOUT` walau aktif): 401 dan cookie dihapus.
4. Cookie `grimoire_session` kosong atau berisi 10 ribu karakter: 401, tanpa query DB berat (panjang ditolak lebih dulu).
5. Database mati saat request ber-cookie: 500 `INTERNAL` generik (bukan 401), isi token tidak muncul di log atau respons.
6. Modul auth tidak ter-register (konfigurasi salah): request ber-cookie 500 (gagal tertutup, perilaku T00), bukan anonim.

## Request/Response body

Tidak ada HTTP langsung. Kontrak bus (Go, bukan JSON di wire), tipe di `lib/contracts/core/auth/resolvesession` (tidak diubah):

```go
// Sukses
resolvesession.Request{Token: "k3J9xQ2mV8bT5nR7cYhL1pWdZ4sA6uEoGfIjKqMtNvB"} // 43 karakter base64url
resolvesession.Response{Found: true, UserID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: false}

// Tidak ditemukan / kedaluwarsa / idle / user nonaktif (BUKAN error)
resolvesession.Response{Found: false}

// Token kosong atau panjang salah: Found=false tanpa query DB
resolvesession.Request{Token: ""} // -> resolvesession.Response{Found: false}, err == nil

// Request salah tipe (rule 6.9): error membungkus bus.ErrBadRequestType
bus.Call(ctx, "core.auth.resolve-session", "string-salah")
// err: "bus: unexpected request type: want=resolvesession.Request got=string"

// Kegagalan infra (DB): error terbungkus, tanpa token/hash
// err: "resolvesession: cari sesi: <driver error>"
```

Dampak ke HTTP lewat middleware `Session` T00 (bukan bagian usecase ini, hanya sebagai rujukan):

Found=false pada request ber-cookie: anonim, header `Set-Cookie: grimoire_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`; endpoint terproteksi menjawab:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

Error bus (DB mati, salah tipe) menjadi:

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

Log yang diharapkan saat touch gagal (tanpa data rahasia):

```json
{"level":"WARN","msg":"touch session gagal","error":"...","trace_id":"4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"}
```
