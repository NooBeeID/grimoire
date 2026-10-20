# T01 core/auth - Usecase `updateuser` (PATCH /api/users/{id})

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1 harus sudah ada: migrasi `users`, `base.User.ApplyUpdate`, `base.CheckRoleChange`, `base/store`, `base/audit`, `base.Shared`). Konvensi: `docs/tickets/README.md` dan `rules.txt`. Overview menang bila ada konflik, kecuali dua penyempurnaan yang ditandai "Penyempurnaan terhadap overview" di bawah (urutan lock dan nama package).

Nama package nyata T00 (sudah merge): `grimoire/internal/infra/middleware` (`RequireAuth`, `RequireRole`, `KeyByIP`), `grimoire/internal/infra/http/respond` (`WriteJSON`, `WriteError`, `DecodeJSON`). Overview menyebut `httpx`; di plan ini dipakai nama nyata. Nama simbol sama. Verifikasi dengan membaca kode sebelum menulis import.

## Metadata

| Field | Nilai |
|---|---|
| Endpoint | `/api/users/{id}` |
| Method | PATCH |
| Version | tanpa prefix versi di URL (`info.version: 0.1.0` di `api/modules/core/auth.yml`) |
| Scope | backend (frontend ditangani `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Feature | `updateuser` (gelombang C, U11) |
| Akses | `RequireAuth` + `RequireRole("super_admin")`; password gate: dicek (user ber-flag `must_change_password` mendapat 403 `PASSWORD_CHANGE_REQUIRED`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Bus | tidak menyediakan; mengonsumsi `core.audit.record` (Call, fail-open) lewat `base/audit` |
| Audit | `user.updated`, status `ok`, hanya bila ada perubahan |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/updateuser/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `updateuser.Init`), `backend/internal/domain/core/internal/auth/auth_integration_test.go` (baris AC6/AC8/AC9), `api/modules/core/auth.yml` + bundle (hanya bila operasi belum ada) |

## Summary Plan

Super Admin mengubah sebagian dari `name`, `email`, `role` sebuah user. Semua field opsional, minimal satu harus ada.

Alur usecase (satu usecase `UpdateUser`, tidak dipecah):
1. Handler memvalidasi body (minimal satu field) dan membangun `Params`.
2. Usecase membuka satu transaksi lewat repository:
   - bila patch memuat `role`: kunci semua super admin aktif (`FOR UPDATE`, urut `id`), lalu kunci target;
   - bila tidak: kunci target saja.
3. Entity menerapkan patch (`User.ApplyUpdate`): normalisasi, validasi, dan hitung `changed`.
4. Bila role berubah: entity `base.CheckRoleChange(actorID, target, newRole, activeSuperAdminCount)` (self lebih dulu, lalu super admin aktif terakhir).
5. Bila `changed` kosong: kembalikan 200 dengan data apa adanya, TANPA UPDATE dan TANPA audit.
6. Bila tidak kosong: `UPDATE users` (name, email, role, updated_at). Unique violation email menjadi 409.
7. Setelah commit: audit `user.updated` (`changed_fields`, plus `role_from`/`role_to` bila role berubah), lalu 200 dengan `UserDTO` terbaru.

Keputusan desain:
- Rule bisnis (normalisasi email, validasi nama/role, guard admin) tetap di `base/`; usecase hanya mengorkestrasi (rule 5.2, 5.6).
- Mengubah role TIDAK mencabut sesi: role dibaca ulang di setiap request oleh `resolvesession` (tiket, bagian Aturan bisnis).
- Mengubah email/nama diri sendiri diizinkan. Menurunkan role diri sendiri dilarang (403 `SELF_ACTION_FORBIDDEN`). Mengirim role yang sama dengan role saat ini (termasuk milik sendiri) adalah no-op, bukan error.
- Menurunkan super admin lain saat masih ada >= 2 super admin aktif diizinkan. Karena actor selalu super admin aktif, `LAST_SUPER_ADMIN` lewat PATCH hanya tercapai saat race (lihat tes race); rule tetap wajib ada dan diuji unit.
- Target nonaktif boleh diubah (nama/email/role). Rule last admin hanya menghitung super admin AKTIF, sehingga mengubah role super admin nonaktif tidak memicu `LAST_SUPER_ADMIN`.
- `null` eksplisit pada field dianggap tidak dikirim (pointer nil). Field tak dikenal ditolak 400 oleh `respond.DecodeJSON` (`DisallowUnknownFields`), sehingga klien tidak bisa mengirim `is_active` atau `password` lewat endpoint ini.
- Urutan evaluasi error: middleware (401/403 CSRF/403 role/403 password gate) -> body tak valid atau kosong (400) -> `{id}` bukan uuid atau tidak ada (404) -> validasi field (400) -> guard admin (403/409) -> unique email (409). 404 sebelum validasi field agar id yang tidak ada tidak membocorkan apa pun tentang validasi.

### Penyempurnaan terhadap overview (urutan lock)

Overview 3.9 menulis urutan: lock target dulu, lalu lock set super admin aktif. Dua transaksi yang menyentuh dua super admin berbeda dengan urutan itu bisa deadlock (tx1 memegang A lalu menunggu B; tx2 memegang B lalu menunggu A). Postgres akan membatalkan salah satu dengan `40P01` dan klien mendapat 500, padahal harusnya salah satu 200 dan lainnya 409.

Plan ini memakai urutan yang konsisten:
1. `SELECT id FROM users WHERE role='super_admin' AND is_active ORDER BY id FOR UPDATE` (set super admin aktif, urut `id`).
2. `SELECT ... FROM users WHERE id=$1 FOR UPDATE` (target; no-op bila sudah terkunci di langkah 1, karena baris yang sama di transaksi yang sama).
3. Count dihitung dari hasil langkah 1 di Go; target dibaca ULANG setelah lock (bukan dari pembacaan sebelumnya).

Hanya dilakukan bila patch memuat `role` (satu-satunya yang butuh guard). Patch tanpa `role` hanya mengunci target, sehingga tidak ada kontensi dengan guard.

Konsekuensi lintas feature (WAJIB disampaikan ke pemilik plan `setuserstatus` dan overview): `setuserstatus` harus memakai urutan yang sama (set dulu urut `id`, baru target) dan overview 3.9 diperbarui. Bila `base/store` F1 sudah menyediakan `LockActiveSuperAdminIDs` tanpa `ORDER BY id`, tambahkan `ORDER BY id` di sana (perubahan satu baris di F1, bukan di folder `updateuser`). Jika F1 sudah terlanjur final dan tidak boleh disentuh dari usecase ini, `updateuser/repository.go` menjalankan SQL kunci yang berurutan itu sendiri; jelaskan di PR.

## Plan

Urutan wajib. Bila prasyarat F1 belum ada, hentikan dan laporkan; jangan menambahkannya di luar allowedPaths.

1. **Cek OpenAPI (sebelum kode, rule 11.6).** Pastikan `api/modules/core/auth.yml` memuat `PATCH /users/{id}`: `operationId: updateUser`, security `sessionCookie`, header `X-Requested-With` wajib, parameter path `id` (uuid), body `UpdateUserRequest` (`minProperties: 1`, `additionalProperties: false`, properti `name`, `email`, `role` dengan enum `super_admin|developer`), respons 200 `$ref User`, 400, 401, 403 (contoh `SELF_ACTION_FORBIDDEN`, `PASSWORD_CHANGE_REQUIRED`, `FORBIDDEN`, `CSRF_REQUIRED`), 404, 409 (contoh `email sudah terdaftar` dan `LAST_SUPER_ADMIN`), 500. Bila belum ada, tambahkan, lalu `make api-bundle` dan `make api-check`.

2. **`updateuser/params.go`** (transport):

   ```go
   package updateuser

   type Params struct {
       ID    string  // dari URL; belum divalidasi uuid (usecase)
       Name  *string
       Email *string
       Role  *string
       Meta  base.RequestMeta
   }

   type body struct {
       Name  *string `json:"name"`
       Email *string `json:"email"`
       Role  *string `json:"role"`
   }

   // parseParams: respond.DecodeJSON(r, &b) (error dikembalikan apa adanya: 400 JSON tidak valid,
   // body wajib diisi, field tak dikenal, atau 413 dari BodyLimit).
   // Bila ketiga field nil -> base.InvalidFields(base.FieldErrors{"_": "minimal satu field harus diisi"}).
   // ID = chi.URLParam(r, "id"). Meta = {IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}.
   func parseParams(r *http.Request) (Params, error)
   ```

   `truncate` adalah helper privat milik feature (jangan impor dari feature lain, rule 4.3). Tanpa trimming di sini; normalisasi dilakukan entity.

3. **`updateuser/usecase.go`** (orkestrasi saja):

   ```go
   // Repository dipenuhi repository.go. Seluruh SQL ada di sana.
   type Repository interface {
       // InTx menjalankan fn dalam satu transaksi (db.BeginTx). Dipanggil dari repository, bukan usecase.
       InTx(ctx context.Context, fn func(ctx context.Context, tx TxRepository) error) error
   }

   type TxRepository interface {
       // LockActiveSuperAdminIDs: ... WHERE role='super_admin' AND is_active ORDER BY id FOR UPDATE.
       LockActiveSuperAdminIDs(ctx context.Context) ([]string, error)
       // LockUserByID: SELECT ... FOR UPDATE; (zero, false, nil) bila tidak ada.
       LockUserByID(ctx context.Context, id string) (base.User, bool, error)
       // UpdateUserFields: UPDATE users SET name,email,role,updated_at WHERE id.
       // 23505 pada users_email_key -> base.ErrEmailTaken.
       UpdateUserFields(ctx context.Context, u base.User) error
   }

   type Usecase struct {
       repo  Repository
       audit audit.Recorder
       clock base.Clock
   }
   func NewUsecase(repo Repository, rec audit.Recorder, clock base.Clock) *Usecase

   type Result struct {
       User    base.User
       Changed []string
   }

   // UpdateUser:
   //  1. actor, ok := reqctx.IdentityFrom(ctx); !ok -> errs.New(CodeUnauthorized, "perlu login") (defensif).
   //  2. uuid.Parse(p.ID) gagal -> base.ErrUserNotFound (tanpa query).
   //  3. repo.InTx:
   //       a. bila p.Role != nil: ids := tx.LockActiveSuperAdminIDs(ctx)
   //       b. target, found := tx.LockUserByID(ctx, p.ID); !found -> base.ErrUserNotFound
   //       c. before := target
   //          changed, err := target.ApplyUpdate(base.UserPatch{Name: p.Name, Email: p.Email, Role: p.Role})
   //          err (FieldErrors) -> base.InvalidFields(fe)
   //       d. bila slices.Contains(changed, "role"):
   //            err := base.CheckRoleChange(actor.UserID, before, target.Role, len(ids))
   //            err != nil -> return err   // *errs.Error siap pakai (403 SELF_ACTION_FORBIDDEN / 409 LAST_SUPER_ADMIN)
   //          CATATAN: CheckRoleChange menerima target dengan Role LAMA (before), newRole = target.Role (baru).
   //       e. len(changed) == 0 -> simpan hasil, return nil (tanpa UPDATE)
   //       f. target.UpdatedAt = u.clock().UTC()
   //          err := tx.UpdateUserFields(ctx, target)
   //          errors.Is(err, base.ErrEmailTaken) -> base.EmailConflict()
   //          err lain -> fmt.Errorf("updateuser: update: %w", err)
   //  4. err != nil -> return (error dibungkus "updateuser: tx: %w" hanya untuk error non-*errs.Error).
   //  5. len(changed) == 0 -> return Result{User: target} (tanpa audit).
   //  6. audit.Record(ctx, audit.Event{
   //         Action: "user.updated", TargetType: "user", Target: target.ID, Status: "ok", Meta: p.Meta,
   //         Args: {"changed_fields": changed, "role_from": before.Role, "role_to": target.Role}  // role_* hanya bila "role" in changed
   //     })   // actor nil -> auditor memakai reqctx identity; fail-open
   //  7. return Result{User: target, Changed: changed}
   func (u *Usecase) UpdateUser(ctx context.Context, p Params) (Result, error)
   ```

   Aturan penting:
   - Nilai `ids` dihitung dari baris yang terkunci (READ COMMITTED + `FOR UPDATE` membaca ulang baris terbaru setelah menunggu lock). Jangan hitung count dengan query agregat terpisah.
   - `ApplyUpdate` membandingkan nilai ternormalisasi dengan nilai tersimpan: `email` yang hanya berbeda huruf besar/kecil atau spasi dari nilai tersimpan TIDAK dihitung berubah. `name` dibandingkan setelah trim.
   - Urutan `changed` selalu `["name","email","role"]` (subset). Audit memakai urutan yang sama sehingga deterministik di test.
   - Audit TIDAK memuat nilai email atau nama baru/lama; hanya nama field (plus role lama/baru). Email tidak perlu disalin ke audit karena ada di tabel users; ini mengurangi PII di log audit.
   - Audit dikirim setelah commit (di luar `InTx`) dan hanya untuk perubahan nyata (AC9: satu aksi satu entri; no-op nol entri).

4. **`updateuser/repository.go`** (satu-satunya yang menyentuh SQL, rule 5.3):

   ```go
   type repository struct {
       db    *db.DB
       store *store.Store // query bersama di base/store
   }
   func NewRepository(d *db.DB) Repository

   func (r *repository) InTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
       return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
           return fn(ctx, &txRepository{store: r.store.WithTx(tx), q: tx})
       })
   }
   ```

   `txRepository`:
   - `LockActiveSuperAdminIDs` dan `LockUserByID` mendelegasikan ke `store.Store` (query identik dipakai `setuserstatus`). Lihat catatan urutan lock di atas tentang `ORDER BY id`.
   - `UpdateUserFields` adalah query milik feature ini, sehingga hidup di `repository.go` feature (bukan di store):

     ```go
     const updateUserFieldsSQL = `
     UPDATE users
        SET name = $2, email = $3, role = $4, updated_at = $5
      WHERE id = $1`
     ```

     `ExecContext`; `RowsAffected() != 1` -> error internal (baris sudah dikunci, jadi tidak boleh terjadi). Deteksi unique: `var pgErr *pgconn.PgError; errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key"` -> `base.ErrEmailTaken`. Constraint lain menjadi error internal terbungkus.
   - Menulis semua kolom (bukan hanya yang berubah) dipilih agar SQL satu bentuk, statis, dan aman (tanpa string builder). Baris sudah terkunci dan entity sudah memuat nilai final.

5. **`updateuser/handler.go`**:

   ```go
   type Handler struct{ uc *Usecase }
   func NewHandler(uc *Usecase) *Handler

   // Patch: p, err := parseParams(r); err -> respond.WriteError(w, r, err).
   // res, err := h.uc.UpdateUser(r.Context(), p); err -> respond.WriteError(w, r, err).
   // sukses: respond.WriteJSON(w, http.StatusOK, res.User.ToResponse())   // UserDTO tanpa pembungkus, tanpa hash
   func (h *Handler) Patch(w http.ResponseWriter, r *http.Request)
   ```

   Tidak ada `Cache-Control: no-store` khusus (tidak ada secret di respons).

6. **`updateuser/init.go`**:

   ```go
   func Init(d app.Deps, s base.Shared) {
       uc := NewUsecase(NewRepository(d.DB), audit.New(d.Bus, d.Log), s.Clock)
       h := NewHandler(uc)
       d.API.With(middleware.RequireAuth(), middleware.RequireRole(constants.RoleSuperAdmin /* "super_admin" */)).
           Patch("/users/{id}", h.Patch)
   }
   ```

   Sesuaikan prefix dengan cara `d.API` di-mount T00 (hasil akhir `/api/users/{id}`; `TestRoutesRegistered` memverifikasi). Jangan membuat konstanta role baru bila `resolvesession.RoleSuperAdmin` (kontrak) sudah ada; pakai itu. `RequireRole` dipasang SETELAH `RequireAuth`. Tidak ada rate limit khusus.

7. **`auth/base.go`**: tambah satu baris `updateuser.Init(d, shared)` di `Register`.

8. **Test** (bagian Test Scenario). Tulis test unit entity lebih dulu bila `ApplyUpdate`/`CheckRoleChange` dari F1 belum mencakup kasus yang tercantum; tambahkan kasus yang kurang di `base/user_test.go` / `base/adminguard_test.go` hanya jika F1 yang mengerjakannya belum merge. Jika sudah merge dan kasus kurang, laporkan, jangan mengubah file F1 dari PR ini.

9. **Verifikasi**: `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, archtest `TestT00_AC4_RepoIsClean` (updateuser tidak meng-import feature lain; SQL hanya di `repository.go`), `make api-check`.

### Kontrak bus

Tidak menyediakan endpoint bus baru. Mengonsumsi `core.audit.record` (mode Call, fail-open) lewat `base/audit`. Request yang dihasilkan (sesuai overview 5.5):

```go
record.Request{
    ActorType: "user", ActorID: "0b7c...", ActorLabel: "admin@tim.id",
    Action: "user.updated", TargetType: "user", Target: "7d1e...",
    Args:   map[string]any{"changed_fields": []string{"name", "role"}, "role_from": "developer", "role_to": "super_admin"},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c...",
}
```

## Migration

Tidak ada. Memakai tabel `users` dari `backend/migrations/core/20261021090000000_create_auth_tables.sql` (F1): constraint `users_email_key` (citext unique), `users_role_check`, `users_name_len`. Tidak ada indeks baru; update berdasarkan primary key.

## Changes

Baru:
- `backend/internal/domain/core/internal/auth/updateuser/init.go`
- `backend/internal/domain/core/internal/auth/updateuser/handler.go`
- `backend/internal/domain/core/internal/auth/updateuser/params.go`
- `backend/internal/domain/core/internal/auth/updateuser/usecase.go`
- `backend/internal/domain/core/internal/auth/updateuser/repository.go`
- `backend/internal/domain/core/internal/auth/updateuser/usecase_test.go` (unit, repository palsu)
- `backend/internal/domain/core/internal/auth/updateuser/repository_integration_test.go` (Postgres nyata)
- `backend/internal/domain/core/internal/auth/updateuser/handler_test.go` (HTTP, params, status mapping)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `updateuser.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go`: baris `user.updated` di tabel AC9 (dan AC8), kasus AC6 sisi PATCH.
- `api/modules/core/auth.yml` dan `api/openapi.yml` (hasil bundle): hanya bila operasi `updateUser` belum ada.
- (Tindak lanjut lintas feature, bukan bagian PR ini) `base/store` `LockActiveSuperAdminIDs` + `ORDER BY id`; overview 3.9 dan plan `setuserstatus` diselaraskan ke urutan lock di atas.

## Test Scenario

Penamaan mengikuti rule 13.4. Hasher/argon2 tidak dipakai di feature ini, jadi test cepat.

### Unit usecase (`usecase_test.go`, repository dan auditor palsu, clock tetap)
- `TestUpdateUser_ChangesNameOnly`: `changed=["name"]`, `UpdateUserFields` dipanggil sekali dengan `updated_at = clock`, audit sekali dengan `changed_fields=["name"]` dan tanpa `role_from/role_to`.
- `TestUpdateUser_ChangesAllFields`: `changed=["name","email","role"]`, `role_from/role_to` terisi.
- `TestUpdateUser_NoChange_NoWriteNoAudit`: patch sama dengan data tersimpan (termasuk email beda huruf besar dan nama dengan spasi tepi) -> tanpa `UpdateUserFields`, tanpa audit, `UpdatedAt` tidak berubah, hasil sukses.
- `TestUpdateUser_SameRoleSelf_IsNoOp`: actor mengirim role sendiri yang sama -> sukses, tanpa error self-action.
- `TestUpdateUser_InvalidID_NotFoundWithoutQuery`: id `"bukan-uuid"` -> `ErrUserNotFound`, repository tidak disentuh.
- `TestUpdateUser_TargetMissing_NotFound`.
- `TestUpdateUser_ValidationErrorsAggregated`: `name=" "`, `email="a@"`, `role="owner"` -> 400 dengan `details.fields` memuat ketiga kunci; tanpa UPDATE, tanpa audit.
- `TestUpdateUser_SelfDemote_Forbidden`: actor == target, super_admin -> developer -> `FORBIDDEN` + `details.reason="SELF_ACTION_FORBIDDEN"`; tanpa UPDATE, tanpa audit.
- `TestUpdateUser_DemoteLastSuperAdmin_Conflict`: lock set mengembalikan 1 id, target super_admin aktif bukan actor -> `CONFLICT` + `LAST_SUPER_ADMIN`.
- `TestUpdateUser_DemoteOtherAdminWhenTwo_OK`: set berisi 2 id -> sukses.
- `TestUpdateUser_DemoteInactiveSuperAdmin_OK`: target super_admin nonaktif, set hanya berisi actor -> sukses (count tidak menghitung target nonaktif).
- `TestUpdateUser_PromoteDeveloper_OK`: developer -> super_admin tidak butuh guard.
- `TestUpdateUser_EmailConflict`: repository mengembalikan `ErrEmailTaken` -> `CONFLICT "email sudah terdaftar"`, tanpa audit.
- `TestUpdateUser_LocksSuperAdminSetOnlyWhenRolePresent`: tanpa `role` di patch, `LockActiveSuperAdminIDs` tidak dipanggil; dengan `role`, dipanggil SEBELUM `LockUserByID` (urutan dicatat penghitung panggilan).
- `TestUpdateUser_AuditArgsHaveNoEmailOrName`: `Args` tidak memuat nilai email atau nama.
- `TestUpdateUser_InfraErrorWrapped`: error DB menjadi error terbungkus non-`*errs.Error` (500 generik di handler), audit tidak dikirim.

### Handler (`handler_test.go`)
- `TestParseParams_EmptyObject`: `{}` -> 400 `fields["_"]`.
- `TestParseParams_AllNull`: `{"name":null}` -> 400 yang sama.
- `TestParseParams_UnknownField`: `{"is_active":false}` dan `{"password":"x"}` -> 400.
- `TestParseParams_MalformedJSON` dan body kosong -> 400.
- `TestHandler_Patch_Returns200WithUserDTO`: respons tidak memuat `password_hash`.
- `TestHandler_Patch_ErrorStatusMapping`: 403 untuk self, 409 untuk last admin, 409 untuk email, 404 untuk id jelek.

### Integrasi repository (`repository_integration_test.go`, `db.TestDB(t)`)
- `TestRepo_UpdateUserFields_Persists`: nilai tersimpan, `created_at` tidak berubah, `updated_at` terisi.
- `TestRepo_UpdateUserFields_EmailUniqueCaseInsensitive`: update ke `A@X.id` saat `a@x.id` dimiliki user lain -> `ErrEmailTaken`.
- `TestRepo_UpdateUserFields_RoleCheckConstraint`: role di luar enum ditolak DB (lapisan terakhir; entity sudah menolak lebih awal).
- `TestRepo_InTx_RollbackOnError`: error di callback membatalkan UPDATE.
- `TestRepo_LockOrder_NoDeadlock`: dua transaksi paralel (A menurunkan B, B menurunkan A) lewat `InTx` dengan urutan set-lalu-target tidak menghasilkan `40P01`; diulang 50 kali.

### Integrasi HTTP lintas module (`auth_integration_test.go`, router lengkap, bus nyata, audit handler palsu)
- `TestT01_AC6_UpdateUser_CannotDemoteSelf`: admin PATCH diri sendiri `{"role":"developer"}` -> 403 `SELF_ACTION_FORBIDDEN`; role tetap `super_admin`; tidak ada audit.
- `TestT01_AC6_UpdateUser_LastAdminRace`: admin A dan B (hanya dua super admin aktif), masing-masing dengan sesi sendiri; A PATCH role B -> developer dan B PATCH role A -> developer bersamaan (goroutine, disinkronkan `sync.WaitGroup` + barrier). Karena targetnya orang lain, hasil yang benar adalah tepat satu 200 dan satu 409 `LAST_SUPER_ADMIN`; di akhir tepat satu super admin aktif. Diulang 20 kali dengan database bersih tiap iterasi.
- `TestT01_AC7_UpdateUser_DeveloperForbidden`: developer PATCH `/api/users/{id}` -> 403 `FORBIDDEN`, DB tidak berubah.
- `TestT01_AC4_UpdateUser_PasswordGate`: super admin ber-flag `must_change_password` -> 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestT01_UpdateUser_RoleChangeTakesEffectWithoutRelogin`: developer D login (sesi hidup), admin PATCH D -> `super_admin`; request D berikutnya ke `GET /api/users` (cookie lama) -> 200. Sesi D tidak dihapus (`COUNT(*) FROM sessions` tetap).
- `TestT01_UpdateUser_EmailChangeThenLoginWithNewEmail`: setelah PATCH email, login email lama gagal 401, email baru berhasil.
- `TestT01_UpdateUser_NoCSRFHeader`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
- `TestT01_AC9_UpdateUser_OneAuditEntry`: tabel kasus: (a) ubah name -> 1 entri `user.updated`; (b) ubah name+email+role -> 1 entri dengan `changed_fields` berurutan; (c) PATCH tanpa perubahan -> 0 entri; (d) ditolak (409/403/400/404) -> 0 entri.
- `TestT01_AC8_UpdateUser_NothingSensitiveInLogs`: log dan `record.Request` tidak memuat `password`, hash PHC, atau token sesi.
- `TestAudit_UnregisteredDoesNotFailUpdate`: tanpa handler audit, PATCH tetap 200 dan log berisi `audit record gagal`.

## E2E Scenarios

Playwright terhadap binary (UI edit pengguna ada di `frontend-plan.md`; skenario di sini menguji alur sampai API/UI).

Positif:
1. Admin membuka `/admin/users`, mengedit nama developer, simpan; baris tabel diperbarui tanpa reload penuh; entri audit `user.updated` dengan `changed_fields=["name"]` terlihat di audit log (bila T02 sudah ada).
2. Admin mengubah role developer menjadi `super_admin`; developer (sesi aktif, tab lain) me-refresh halaman dan menu Users muncul tanpa login ulang.
3. Admin mengubah email developer; developer login dengan email baru berhasil, email lama ditolak dengan pesan generik.
4. Admin menyimpan form edit tanpa mengubah apa pun; UI menampilkan sukses, `updated_at` tidak berubah, dan tidak ada entri audit baru.
5. Admin mengubah nama dirinya sendiri; berhasil, header UI memperlihatkan nama baru setelah `GET /api/auth/me`.

Negatif:
1. Admin menurunkan role dirinya sendiri (lewat UI yang dinonaktifkan, dan lewat `fetch` manual): API 403 `SELF_ACTION_FORBIDDEN`, role tidak berubah.
2. Dua admin saling menurunkan secara bersamaan (dua tab/dua konteks browser): satu berhasil, satu mendapat 409 `LAST_SUPER_ADMIN` dengan pesan jelas; sistem tetap punya satu super admin aktif.
3. Mengubah email ke email milik user lain (beda huruf besar/kecil): 409, pesan "email sudah terdaftar" tampil di field email pada form.
4. Nama kosong, email tidak valid, atau role tidak dikenal: 400, pesan per field tampil; tidak ada perubahan data.
5. Developer memanggil `PATCH /api/users/{id}` lewat `fetch`: 403 `FORBIDDEN`.
6. User ber-flag `must_change_password` memanggil PATCH: 403 `PASSWORD_CHANGE_REQUIRED`.
7. `PATCH /api/users/tidak-ada` dan `PATCH /api/users/<uuid-acak>`: 404 `user tidak ditemukan`.
8. Body `{}` atau `{"is_active":false}`: 400.
9. Tanpa cookie sesi: 401; tanpa `X-Requested-With`: 403 `CSRF_REQUIRED`.
10. User target dinonaktifkan oleh admin lain saat form edit terbuka: PATCH tetap 200 (nonaktif boleh diedit); status nonaktif tidak berubah oleh PATCH.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Header wajib: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie `grimoire_session`.

### Request

`PATCH /api/users/7d1e5a2c-3b4f-4d6e-8f90-a1b2c3d4e5f6`

Semua field opsional, minimal satu:

```json
{ "name": "Sari W.", "email": "Sari.W@Tim.ID", "role": "super_admin" }
```

Hanya nama:

```json
{ "name": "Sari W." }
```

### 200 OK (ada perubahan atau tidak ada perubahan; bentuk sama, `UserDTO` tanpa pembungkus)

```json
{
  "id": "7d1e5a2c-3b4f-4d6e-8f90-a1b2c3d4e5f6",
  "email": "sari.w@tim.id",
  "name": "Sari W.",
  "role": "super_admin",
  "is_active": true,
  "must_change_password": false,
  "created_at": "2026-10-21T09:00:00Z",
  "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
  "created_by_label": "admin@tim.id",
  "updated_at": "2026-10-21T10:15:00Z",
  "last_login_at": "2026-10-21T09:10:00Z"
}
```

Tanpa perubahan: isi sama dengan kondisi tersimpan, `updated_at` tidak berubah, tidak ada audit.

### 400 body kosong atau semua field null

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "_": "minimal satu field harus diisi" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### 400 validasi field

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": {
      "fields": {
        "name": "nama wajib diisi",
        "email": "format email tidak valid",
        "role": "role harus super_admin atau developer"
      }
    }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### 400 field tak dikenal

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "JSON tidak valid", "details": null }, "trace_id": "..." }
```

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

### 403 developer

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

### 403 password gate

```json
{ "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null }, "trace_id": "..." }
```

### 403 menurunkan role sendiri

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "tidak boleh menonaktifkan atau menurunkan role akun sendiri",
    "details": { "reason": "SELF_ACTION_FORBIDDEN" }
  },
  "trace_id": "..."
}
```

### 404 id tidak ada atau bukan uuid

```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null }, "trace_id": "..." }
```

### 409 super admin aktif terakhir

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "minimal harus ada satu super admin aktif",
    "details": { "reason": "LAST_SUPER_ADMIN" }
  },
  "trace_id": "..."
}
```

### 409 email sudah dipakai

```json
{ "error": { "code": "CONFLICT", "message": "email sudah terdaftar", "details": null }, "trace_id": "..." }
```

### 500 (kegagalan infra, generik)

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null }, "trace_id": "..." }
```

### Audit (`core.audit.record`, bukan JSON di wire)

```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "admin@tim.id",
    Action: "user.updated", TargetType: "user", Target: "7d1e5a2c-3b4f-4d6e-8f90-a1b2c3d4e5f6",
    Args:   map[string]any{"changed_fields": []string{"name", "email", "role"}, "role_from": "developer", "role_to": "super_admin"},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33",
}
```
