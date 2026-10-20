# T01 core/auth — Usecase `setuserstatus`

Plan rinci satu usecase. Wajib mengikuti `docs/superpowers/plans/core-auth/overview.md` (selanjutnya "overview"); bila bertentangan, overview menang sampai diperbarui, kecuali butir S1 di bawah yang mengusulkan koreksi overview 3.9. Sumber aturan: `rules.txt`, `docs/tickets/README.md`, `docs/tickets/T01-core-auth.md`.

Catatan konteks: belum ada `CLAUDE.md`, `.claude/rules/`, maupun knowledge doc module auth.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (plan ini hanya backend; UI di `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Usecase / folder | `setuserstatus` (SATU folder, DUA route) |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Endpoint | `/api/users/{id}/deactivate` dan `/api/users/{id}/activate` |
| Method | POST (keduanya) |
| Version | tanpa prefix versi di URL (`api/modules/core/auth.yml` `info.version: 0.1.0`) |
| Auth | `RequireAuth` + `RequireRole(super_admin)` |
| Password gate | tidak exempt (user ber-flag `must_change_password` mendapat 403 `PASSWORD_CHANGE_REQUIRED`) |
| Gelombang | C (setelah F1, `login`, `resolvesession`) |
| Endpoint bus | tidak menyediakan. Mengonsumsi `core.audit.record` (Call, via `base/audit`) |
| Job / CLI | tidak ada |
| AC | AC5, AC6 (bagian: self dan last admin; `updateuser` memegang bagian role) |
| Bergantung pada | F1 overview (config, `base/*`, `base/store`, `base/audit`, migrasi `users`/`sessions`), `resolvesession` (untuk menguji AC5 end to end) |

---

## 2. Summary Plan

Super Admin menonaktifkan atau mengaktifkan user lain.

Deactivate, dalam SATU transaksi:
1. Kunci semua super admin aktif (urut `id`), lalu kunci baris target.
2. Rule entity `CheckDeactivate`: bukan diri sendiri, bukan super admin aktif terakhir.
3. `UPDATE users SET is_active=false`, lalu `DELETE FROM sessions WHERE user_id=$target` (semua sesi).
4. Setelah commit: audit `user.deactivated` dengan `revoked_sessions`.

Activate: kunci baris target, `UPDATE users SET is_active=true`, audit `user.activated` setelah commit. Tidak ada penjaga admin (mengaktifkan tidak bisa melanggar aturan).

Idempoten: bila status sudah sama, tidak ada tulis, tidak ada audit, respons tetap 200 dengan user apa adanya.

Respons sukses: 200 `UserDTO` terbaru. Efek AC5: request berikutnya dengan cookie lama user target 401 seketika karena sesinya terhapus di transaksi yang sama dan `resolvesession` juga menolak user nonaktif.

Keputusan desain khusus usecase ini:

| # | Keputusan | Alasan |
|---|---|---|
| S1 | Urutan lock deactivate: `LockActiveSuperAdminIDs` (ORDER BY id) DULU, baru `LockUserByID(target)`. Ini mengoreksi overview 3.9 yang mengunci target dulu | Bila target dikunci dulu, dua admin A dan B yang saling menonaktifkan membentuk siklus (tx1 pegang B minta A, tx2 pegang A minta B) dan Postgres membatalkan salah satu dengan `40P01 deadlock_detected` (500), bukan `LAST_SUPER_ADMIN`. Dengan set admin dikunci lebih dulu dalam urutan `id` yang tetap, tx kedua menunggu, lalu READ COMMITTED mengevaluasi ulang predikat `is_active`, melihat hanya satu admin tersisa, dan rule menghasilkan `LAST_SUPER_ADMIN`. Perubahan overview 3.9 dilakukan oleh PR ini (satu paragraf) |
| S2 | Activate hanya mengunci target, tanpa set admin | Tidak ada rule yang bergantung pada jumlah admin; mengurangi kontensi |
| S3 | Satu `Usecase.Execute(ctx, Input{TargetID, Active})`, dua method handler tipis | Dua route berbagi 90 persen alur; memecahnya artifisial. Arah ditentukan oleh `Input.Active` |
| S4 | Tidak ada body request; respons `UserDTO` tanpa pembungkus | Konsisten dengan `updateuser` dan `getuser` |
| S5 | Entity user diubah lewat metode murni `ApplyStatus(active, now) bool` di `base/user.go` (tambahan aditif), dan hasilnya dipakai sebagai respons tanpa SELECT ulang | Rule status ada di entity (rules.txt 5.5); hemat satu query |
| S6 | Target tidak ada atau `{id}` bukan uuid: 404 `NOT_FOUND` "user tidak ditemukan" | Sama dengan overview 3.12 |
| S7 | Audit setelah commit, fail-open (`base/audit`) | Overview 5.5 |
| S8 | Mengaktifkan kembali TIDAK memulihkan sesi lama dan tidak mengubah `must_change_password` | User harus login ulang; flag tetap apa adanya |

---

## 3. Plan (langkah berurutan)

Aturan umum: TDD per langkah (test dulu). Tidak mengubah signature publik T00. Nama simbol T00 sesuai overview bagian 0.

### Langkah 1 — Prasyarat dari F1 (verifikasi, jangan duplikasi)

Pastikan sudah ada: tabel `users` dan `sessions`; `base.User`, `base.CheckDeactivate`, `base.ErrSelfAction`, `base.ErrLastSuperAdmin`, `base.ErrUserNotFound`, `base.UserDTO`, `base.RequestMeta`, `base.Clock`; `store.Store` dengan `WithTx`, `LockUserByID`, `LockActiveSuperAdminIDs`, `DeleteSessionsByUser`; `audit.Recorder`.

Dua penyesuaian kecil pada artefak F1 bila belum terpenuhi (tambahan aditif, test di file F1 masing-masing):
1. `store.LockActiveSuperAdminIDs`: query harus `SELECT id FROM users WHERE role='super_admin' AND is_active ORDER BY id FOR UPDATE` (urutan `id` wajib, S1).
2. `store.LockUserByID`: id bukan uuid mengembalikan `(zero, false, nil)` tanpa query (sama dengan `FindUserByID`).

### Langkah 2 — Rule entity: `ApplyStatus`

Tambahan di `base/user.go` (murni, tanpa I/O):

```go
// ApplyStatus mengubah status aktif. Mengembalikan true bila ada perubahan.
// Status sama -> false, user tidak berubah (UpdatedAt tidak disentuh).
func (u *User) ApplyStatus(active bool, now time.Time) bool
```

Test unit `user_test.go`: `TestUser_ApplyStatus` (false->true, true->false, sama -> `changed=false` dan `UpdatedAt` tetap).

`CheckDeactivate` sudah ada di `base/adminguard.go` (overview 4.2); jangan diubah. Perilakunya yang dipakai:
- `!target.IsActive` -> nil (idempoten)
- `target.ID == actorID` -> `ErrSelfAction`
- `target.Role == super_admin && count <= 1` -> `ErrLastSuperAdmin`

### Langkah 3 — Folder `setuserstatus/`

Path: `backend/internal/domain/core/internal/auth/setuserstatus/`. File: `init.go`, `handler.go`, `usecase.go`, `repository.go`, dan test. Tidak ada `params.go` karena tidak ada body request (S4); id diambil di handler.

**`repository.go`** (satu-satunya yang menyentuh SQL, lewat `store.Store` dan satu query milik feature):

```go
type Repo struct {
    db    db.DB
    store *store.Store
}
func NewRepo(d db.DB) *Repo

// Tx adalah operasi di dalam transaksi. Hanya valid selama fn berjalan.
type Tx struct{ store *store.Store /* WithTx(tx) */ ; tx *sqlx.Tx }

// InTx membuka transaksi (db.BeginTx dipanggil DI SINI, bukan di usecase), menjalankan fn, commit bila nil, rollback bila error/panic.
func (r *Repo) InTx(ctx context.Context, fn func(*Tx) error) error

func (t *Tx) LockActiveSuperAdminIDs(ctx context.Context) ([]string, error)       // store, FOR UPDATE ORDER BY id
func (t *Tx) LockUser(ctx context.Context, id string) (base.User, bool, error)    // store.LockUserByID
// UPDATE users SET is_active=$2, updated_at=$3 WHERE id=$1
func (t *Tx) SetActive(ctx context.Context, id string, active bool, now time.Time) error
func (t *Tx) DeleteSessions(ctx context.Context, userID string) (int64, error)    // store.DeleteSessionsByUser(ctx, userID, nil)
```

Catatan SQL: semua berparameter, kolom eksplisit, tanpa `SELECT *`. `SetActive` memeriksa `RowsAffected == 1`, selain itu error (baris sudah terkunci, jadi 0 baris berarti bug).

**`usecase.go`**

```go
type Tx interface {
    LockActiveSuperAdminIDs(ctx context.Context) ([]string, error)
    LockUser(ctx context.Context, id string) (base.User, bool, error)
    SetActive(ctx context.Context, id string, active bool, now time.Time) error
    DeleteSessions(ctx context.Context, userID string) (int64, error)
}
type Repository interface {
    InTx(ctx context.Context, fn func(Tx) error) error
}
```

`Repo.InTx` mengembalikan `*Tx` konkret; di `init.go` dipasang adaptor tipis agar `*Repo` memenuhi `Repository` (fungsi pembungkus `func(Tx) error` -> `func(*Tx) error`). Usecase hanya melihat interface (rule 5.6).

```go
type Input struct {
    TargetID string
    Active   bool           // true = activate, false = deactivate
    Meta     base.RequestMeta
}
type Usecase struct {
    repo  Repository
    audit audit.Recorder
    clock base.Clock
}
func (u *Usecase) Execute(ctx context.Context, in Input) (base.UserDTO, error)
```

Urutan `Execute` (orkestrasi saja):
1. `actor, ok := reqctx.IdentityFrom(ctx)`; tidak ada: `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `now := u.clock()`; siapkan `var user base.User; var changed bool; var revoked int64`.
3. `err := repo.InTx(ctx, func(tx Tx) error { ... })`:
   - Bila `!in.Active`: `ids, err := tx.LockActiveSuperAdminIDs(ctx)` (S1, error dibungkus `fmt.Errorf("setuserstatus: kunci super admin: %w", err)`).
   - `user, found, err := tx.LockUser(ctx, in.TargetID)`; `!found` -> `base.ErrUserNotFound`.
   - Bila `!in.Active`: `base.CheckDeactivate(actor.UserID, user, len(ids))`; error dikembalikan apa adanya (sudah `*errs.Error`).
   - `changed = user.ApplyStatus(in.Active, now)`; `!changed` -> return nil (tanpa tulis).
   - `tx.SetActive(ctx, user.ID, in.Active, now)`.
   - Bila `!in.Active`: `revoked, err = tx.DeleteSessions(ctx, user.ID)`.
4. Error dari transaksi dikembalikan; error infra dibungkus `fmt.Errorf("setuserstatus: transaksi: %w", err)`.
5. Setelah commit, hanya bila `changed`: `audit.Record(ctx, audit.Event{Action: "user.deactivated" | "user.activated", TargetType: "user", Target: user.ID, Args: {"revoked_sessions": revoked} | {}, Status: "ok", Meta: in.Meta})`. Actor dari ctx (`Actor` nil). Label actor diisi `audit` dari `Identity.Email`.
6. Return `user.ToResponse()`.

Catatan: karena `CheckDeactivate` mengembalikan nil untuk target yang sudah nonaktif, urutan "idempoten sebelum self-check" otomatis: menonaktifkan akun yang sudah nonaktif adalah no-op 200. Aktor selalu aktif, jadi self-deactivate selalu terdeteksi sebagai `ErrSelfAction`.

**`handler.go`**

```go
type Handler struct{ uc *Usecase }
func (h *Handler) Deactivate(w http.ResponseWriter, r *http.Request)
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request)
func (h *Handler) serve(w http.ResponseWriter, r *http.Request, active bool)
```

`serve`: `id := chi.URLParam(r, "id")`; `meta := base.RequestMeta{IP: httpx.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`; `dto, err := h.uc.Execute(...)`; error -> `httpx.WriteError(w, r, err)`; sukses -> `httpx.WriteJSON(w, r, http.StatusOK, dto)`. Body request diabaikan (tidak dibaca).

**`init.go`**

```go
func Init(d app.Deps, s base.Shared) {
    repo := NewRepo(d.DB)
    uc := &Usecase{repo: txAdapter{repo}, audit: audit.New(d.Bus, d.Log), clock: s.Clock}
    h := &Handler{uc: uc}
    admin := d.API.With(httpx.RequireAuth, httpx.RequireRole(resolvesession.RoleSuperAdmin))
    admin.Post("/users/{id}/deactivate", h.Deactivate)
    admin.Post("/users/{id}/activate", h.Activate)
}
```

(`resolvesession.RoleSuperAdmin` dari `lib/contracts/core/auth/resolvesession`.) Tidak ada rate limit khusus (aksi admin jarang).

### Langkah 4 — Wiring dan OpenAPI

1. `auth/base.go`: tambah satu baris `setuserstatus.Init(d, shared)` di `Register`.
2. `api/modules/core/auth.yml`: pastikan dua operasi ada (seharusnya sudah dari F1), `operationId: deactivateUser` dan `activateUser`, respons 200 `User`, error 401/403/404/409 via `$ref`, contoh `details.reason`. Jalankan `make api-bundle` dan `make api-check`. Bila belum ada, tambahkan SEBELUM kode handler (rule 11.6).
3. `docs/superpowers/plans/core-auth/overview.md` bagian 3.9: ganti urutan lock sesuai S1 (satu paragraf).
4. Tambah baris `user.deactivated`/`user.activated` di tabel test AC9 (`auth_integration_test.go`) dan baris di test AC8 (tidak ada password; usecase ini tidak menyentuh password).

---

## 4. Migration

Tidak ada. Memakai tabel `users` dan `sessions` dari migrasi F1 `backend/migrations/core/20261021100000000_create_auth_tables.sql`. Index `users_role_active_idx (role, is_active)` sudah mendukung query set super admin aktif dan `sessions_user_id_idx` mendukung delete sesi per user.

---

## 5. Changes (per file/folder)

Baru:
- `backend/internal/domain/core/internal/auth/setuserstatus/init.go`
- `backend/internal/domain/core/internal/auth/setuserstatus/handler.go`
- `backend/internal/domain/core/internal/auth/setuserstatus/usecase.go`
- `backend/internal/domain/core/internal/auth/setuserstatus/repository.go`
- `backend/internal/domain/core/internal/auth/setuserstatus/usecase_test.go` (unit, repo dan audit palsu)
- `backend/internal/domain/core/internal/auth/setuserstatus/repository_integration_test.go` (Postgres nyata)
- `backend/internal/domain/core/internal/auth/setuserstatus/handler_test.go` (httptest, usecase palsu)

Diubah:
- `backend/internal/domain/core/internal/auth/base/user.go` dan `user_test.go`: `ApplyStatus`.
- `backend/internal/domain/core/internal/auth/base/store/store.go`: pastikan `ORDER BY id` pada `LockActiveSuperAdminIDs` dan guard uuid pada `LockUserByID` (bila belum); tambah test di `store_integration_test.go`.
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `setuserstatus.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go`: `TestT01_AC5_*`, `TestT01_AC6_*`, baris AC9.
- `api/modules/core/auth.yml`, `api/openapi.yml` (generated): dua operasi.
- `docs/superpowers/plans/core-auth/overview.md` 3.9: urutan lock (S1).

Tidak diubah: migrasi, config, kontrak bus, konstanta, infra.

---

## 6. Test Scenario

Penamaan `TestT01_ACn_...` untuk AC (rule 13.4). Test integrasi memakai `db.TestDB(t)` dan migrasi `migrations.FS`.

### Unit entity (tanpa mock; sebagian sudah di F1)
- `TestUser_ApplyStatus`: lihat Langkah 2.
- `TestCheckDeactivate` (tabel; F1) dan pastikan kasus ini ada: target diri sendiri; target super admin aktif terakhir (count=1); super admin kedua (count=2) lolos; developer lolos; target sudah nonaktif lolos (nil) meski diri sendiri atau count=1; self lebih dulu daripada last-admin (self + count=1 -> `ErrSelfAction`).

### Unit usecase (`usecase_test.go`, repo/tx palsu in-memory dan audit pencatat)
- `TestExecute_Deactivate_Success`: memanggil `LockActiveSuperAdminIDs` SEBELUM `LockUser` (urutan dicatat), `SetActive(false)`, `DeleteSessions`, satu audit `user.deactivated` dengan `revoked_sessions` sesuai; DTO `is_active=false`.
- `TestExecute_Activate_Success`: tidak memanggil `LockActiveSuperAdminIDs` maupun `DeleteSessions`; satu audit `user.activated` dengan Args kosong.
- `TestExecute_Deactivate_Self`: `ErrSelfAction` (403, `details.reason=SELF_ACTION_FORBIDDEN`), tanpa `SetActive`, tanpa audit.
- `TestExecute_Deactivate_LastSuperAdmin`: tx palsu mengembalikan 1 id; target super admin aktif lain; `ErrLastSuperAdmin` (409), tanpa audit.
- `TestExecute_Deactivate_AlreadyInactive_NoOp`: tanpa `SetActive`, tanpa `DeleteSessions`, tanpa audit, DTO dikembalikan.
- `TestExecute_Activate_AlreadyActive_NoOp`: sama.
- `TestExecute_TargetNotFound`: `ErrUserNotFound` (404), tanpa audit.
- `TestExecute_NoIdentity`: 401.
- `TestExecute_InfraErrorWrapped`: error `SetActive` dibungkus `setuserstatus: transaksi: ...`; tidak ada audit; pesan klien generik via `WriteError`.
- `TestExecute_AuditNotCalledOnRollback`: error di `DeleteSessions` -> tidak ada audit.

### Handler (`handler_test.go`)
- `TestHandler_RoutesAndStatus`: `POST /users/{id}/deactivate` dan `/activate` memanggil usecase dengan `Active` yang benar; sukses 200 body `UserDTO` JSON; error `*errs.Error` dipetakan (403, 404, 409).
- `TestHandler_IgnoresBody`: body sampah tidak mempengaruhi.

### Integrasi repository (`repository_integration_test.go`, Postgres nyata)
- `TestRepo_InTx_DeactivateDeletesSessionsAtomically`: user punya 3 sesi; deactivate sukses -> `is_active=false`, 0 sesi, `updated_at` berubah. Error dipaksa di tengah (callback mengembalikan error setelah `SetActive`) -> rollback: user tetap aktif, sesi tetap 3.
- `TestRepo_LockActiveSuperAdminIDs_OrderedByID`: hasil terurut dan hanya yang aktif.
- `TestRepo_NoDeadlock_MutualDeactivate`: dua goroutine (A menonaktifkan B, B menonaktifkan A) memakai `Usecase` + `Repo` nyata dengan identity masing-masing; tepat satu sukses dan satu `LAST_SUPER_ADMIN`; tidak ada error `40P01`; diulang 20 kali (reset data tiap iterasi) untuk menekan flakiness.
- `TestRepo_ActivateDoesNotRestoreSessions`: activate user yang nonaktif -> 0 sesi tetap 0.

### Integrasi lintas module (`auth_integration_test.go`, router lengkap + bus nyata + handler audit palsu)
- `TestT01_AC5_DeactivateKillsSessionImmediately`: user B login (cookie); admin `POST /api/users/{B}/deactivate` -> 200; request B berikutnya (`GET /api/auth/me`) dengan cookie lama -> 401 dan `Set-Cookie ... Max-Age=0`; tabel `sessions` B kosong; B login lagi -> 401 `INVALID_CREDENTIALS` (nonaktif, respons sama dengan password salah).
- `TestT01_AC5_ActivateAllowsLoginAgain`: setelah `/activate`, B login dengan password lama -> 200 (sesi baru).
- `TestT01_AC6_CannotDeactivateSelf`: admin `POST /api/users/{self}/deactivate` -> 403 `FORBIDDEN`, `details.reason=SELF_ACTION_FORBIDDEN`; user tetap aktif; 0 audit.
- `TestT01_AC6_LastSuperAdminRace`: dua super admin A dan B dengan sesi masing-masing, saling menonaktifkan paralel lewat HTTP: tepat satu 200 dan satu 409 `LAST_SUPER_ADMIN`; di akhir tepat satu super admin aktif (query DB).
- `TestT01_AC6_DeactivateDeveloperWhileOneAdmin`: hanya satu super admin aktif, menonaktifkan developer -> 200 (aturan hanya melindungi super admin).
- `TestT01_AC7_DeveloperForbiddenOnStatus`: developer memanggil kedua route -> 403 `FORBIDDEN`; anonim -> 401; user ber-flag `must_change_password` (super_admin) -> 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestT01_AC9_StatusAuditEntries`: deactivate sukses -> tepat satu `record.Request` `user.deactivated` (`ActorType=user`, `ActorLabel` email admin, `TargetType=user`, `Target=<id>`, `Args.revoked_sessions=N`, `Status=ok`); activate -> satu `user.activated`; panggilan kedua yang no-op -> 0 entri tambahan.
- `TestT01_AC8_StatusNoSecrets`: log dan `record.Request` tidak memuat token cookie, hash PHC, atau `$argon2id$`.
- `TestAudit_UnregisteredDoesNotFailStatus`: tanpa handler audit, deactivate tetap 200, log berisi `audit record gagal`.
- `TestStatus_InvalidAndUnknownID`: `{id}` bukan uuid dan uuid acak -> 404 `NOT_FOUND`.
- `TestRoutesRegistered` (milik W) memuat dua route ini.

---

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Admin membuka Admin > Users, memilih developer aktif, klik "Nonaktifkan", konfirmasi; baris berubah menjadi status Nonaktif dan tombol menjadi "Aktifkan".
2. Developer yang sedang login di tab/sesi lain: aksi berikutnya (muat ulang atau klik) menerima 401 dan UI kembali ke `/login` seketika setelah admin menonaktifkan (AC5).
3. Developer nonaktif mencoba login: pesan generik `email atau password salah`.
4. Admin mengaktifkan kembali developer itu; developer bisa login dengan password lama.
5. Mengulang aksi yang sama dari dua tab admin (idempoten): tab kedua tetap sukses tanpa error UI; log audit hanya mencatat satu entri `user.deactivated`.

Negatif:
1. Baris akun admin sendiri: tombol "Nonaktifkan" disembunyikan atau disabled. Dipanggil manual lewat fetch: 403 `SELF_ACTION_FORBIDDEN`.
2. Dua super admin saling menonaktifkan nyaris bersamaan (dua browser): satu berhasil, satu mendapat pesan "minimal harus ada satu super admin aktif" (409); sistem tetap punya satu super admin aktif.
3. Developer memanggil `POST /api/users/{id}/deactivate` secara manual: 403 `FORBIDDEN`.
4. Tanpa header `X-Requested-With`: 403 `CSRF_REQUIRED`.
5. `POST /api/users/00000000-0000-0000-0000-000000000000/deactivate`: 404 `NOT_FOUND`.
6. Super admin ber-flag ganti password memanggil endpoint ini: 403 `PASSWORD_CHANGE_REQUIRED`.

---

## 8. Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire` dan cookie sesi. Kedua endpoint tidak punya body request.

### POST /api/users/{id}/deactivate

Request: tanpa body.

200 (user target setelah dinonaktifkan; juga 200 bila sudah nonaktif sebelumnya, tanpa audit):
```json
{
  "id": "7d1e5a90-3c42-4b7f-8a11-0f6d2c9e4b17",
  "email": "sari@tim.id",
  "name": "Sari",
  "role": "developer",
  "is_active": false,
  "must_change_password": false,
  "created_at": "2026-10-21T09:00:00Z",
  "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
  "created_by_label": "admin@tim.id",
  "updated_at": "2026-10-21T10:30:00Z",
  "last_login_at": "2026-10-21T09:10:00Z"
}
```

403 menonaktifkan diri sendiri:
```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "tidak boleh menonaktifkan atau menurunkan role akun sendiri",
    "details": { "reason": "SELF_ACTION_FORBIDDEN" }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

409 super admin aktif terakhir:
```json
{
  "error": {
    "code": "CONFLICT",
    "message": "minimal harus ada satu super admin aktif",
    "details": { "reason": "LAST_SUPER_ADMIN" }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

404 (tidak ada atau bukan uuid):
```json
{
  "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

403 bukan super admin:
```json
{
  "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

403 flag ganti password aktif:
```json
{
  "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

401 tanpa sesi:
```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### POST /api/users/{id}/activate

Request: tanpa body.

200 (`is_active` true, bentuk sama dengan deactivate):
```json
{
  "id": "7d1e5a90-3c42-4b7f-8a11-0f6d2c9e4b17",
  "email": "sari@tim.id",
  "name": "Sari",
  "role": "developer",
  "is_active": true,
  "must_change_password": false,
  "created_at": "2026-10-21T09:00:00Z",
  "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
  "created_by_label": "admin@tim.id",
  "updated_at": "2026-10-21T11:00:00Z",
  "last_login_at": "2026-10-21T09:10:00Z"
}
```

Error: 401, 403 (`FORBIDDEN` / `PASSWORD_CHANGE_REQUIRED`), 404 sama seperti di atas. Activate tidak punya 409 maupun `SELF_ACTION_FORBIDDEN`. Error CSRF: `403 CSRF_REQUIRED` dari middleware T00.

### Contoh `record.Request` (audit)

Deactivate:
```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "admin@tim.id",
    Action: "user.deactivated", TargetType: "user", Target: "7d1e5a90-3c42-4b7f-8a11-0f6d2c9e4b17",
    Args: map[string]any{"revoked_sessions": 2}, Status: "ok",
    IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33",
}
```

Activate:
```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-...", ActorLabel: "admin@tim.id",
    Action: "user.activated", TargetType: "user", Target: "7d1e5a90-...",
    Args: map[string]any{}, Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c...",
}
```
