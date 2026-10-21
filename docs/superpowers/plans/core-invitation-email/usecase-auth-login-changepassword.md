# T12 core-invitation-email: Usecase `auth-login-changepassword`

Tiket: `docs/tickets/T12-core-invitation-email.md` (K3, K4, kriteria 2 dan 3).
Foundation yang wajib diikuti: `docs/superpowers/plans/core-invitation-email/overview.md` (bagian 3.3, 4.6, 4.8). Bila bertentangan, overview menang.

Plan ini hanya mengubah perilaku dua feature T01 yang sudah ada: `login` dan `changepassword`. Tidak ada feature baru, tidak ada endpoint baru, tidak ada kontrak bus baru. Keduanya satu plan karena membentuk siklus hidup undangan: `login` menegakkan kedaluwarsa, `changepassword` menandai diterima.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Usecase slug | `auth-login-changepassword` |
| Scope | backend (frontend tidak berubah di plan ini; teks bantu login/ganti password ada di `frontend-plan.md`) |
| Domain / Module | core / auth |
| Folder | `backend/internal/domain/core/internal/auth/login`, `backend/internal/domain/core/internal/auth/changepassword` |
| Gelombang | A (setelah foundation, paralel dengan `auth-invitation-markers` dan `management-countmemberships`) |

| Endpoint | Method | Version | Auth | Perubahan |
|---|---|---|---|---|
| `/api/auth/login` | POST | - (aditif perilaku) | publik, rate limit per IP | akun undangan menunggu yang kedaluwarsa dijawab identik kredensial salah |
| `/api/auth/change-password` | POST | - (aditif perilaku) | `RequireAuth`, rate limit per user | undangan kedaluwarsa ditolak; undangan menunggu ditandai diterima dalam tx yang sama |

Endpoint bus: tidak ada yang baru atau berubah. Audit baru yang dicatat oleh plan ini: `user.invitation.accepted` (konstanta `record.ActionUserInvitationAccepted`, ditambahkan di foundation 4.8).

Prasyarat dari foundation (harus sudah ada dan hijau sebelum plan ini dikerjakan):

- Migrasi `user_invitations` (core).
- `base.Invitation` dengan `IsPending`, `IsExpired(now)`, `BlocksLogin(now)` (`base/invitation.go`).
- `store.Store.FindInvitation(ctx, userID) (base.Invitation, bool, error)` dan `store.Store.AcceptInvitation(ctx, userID, now) (bool, error)` (`base/store/store.go`), termasuk `WithTx`.
- `record.ActionUserInvitationAccepted` di `lib/contracts/core/audit/record/actions.go`.

---

## 2. Summary Plan

Siklus hidup (overview 3.3): akun tanpa baris undangan berperilaku persis seperti T01. Akun dengan undangan menunggu boleh login selama `now < expires_at` dan tetap terkena `PasswordChangeGate` karena `must_change_password=true`. Setelah `expires_at` tanpa diterima, login ditolak.

`login`:
- Setelah password cocok dan `CanLogin()` lolos, usecase memuat undangan lewat `Repository.FindInvitation`. Bila `inv.BlocksLogin(now)`, jalur ini diperlakukan sama persis dengan password salah: `RegisterFailure` pada throttle email, audit `auth.login.failed` dengan `reason="invitation_expired"`, dan error `base.ErrInvalidCreds` (401 `INVALID_CREDENTIALS`, pesan, header, tanpa cookie identik). Penolakan ini ikut menaikkan hitungan lockout, sehingga penyerang tidak bisa membedakan akun kedaluwarsa dari password salah, dan tidak bisa menebak password sementara tanpa batas.
- Query undangan hanya dijalankan bila password benar dan akun aktif. Jalur password salah dan akun tidak ada tidak berubah (tidak ada query tambahan, dummy hash tetap dipakai).

`changepassword`:
- Setelah user ditemukan dan `CanLogin()`, usecase memuat undangan. Bila undangan menunggu dan `IsExpired(now)`, kembalikan `base.InvalidFields({"current_password": ...})` (400 `INVALID_ARGUMENT`) tanpa verifikasi hash dan tanpa menyentuh DB tulis. Ini menutup celah sesi yang dibuat sebelum kedaluwarsa (overview 3.3: `resolvesession` tidak berubah).
- `Repo.Change` dalam tx yang sama (setelah `UPDATE users` dan hapus sesi) menjalankan `UPDATE user_invitations SET accepted_at=$now WHERE user_id=$1 AND accepted_at IS NULL` lewat `store.WithTx(tx).AcceptInvitation`, lalu mengembalikan `accepted bool`. `ErrStale` atau error lain me-rollback semuanya, sehingga `accepted_at` tidak pernah terisi tanpa password terganti.
- Usecase mencatat `auth.password.changed` (seperti sekarang), lalu bila `accepted`, mencatat `user.invitation.accepted` setelah commit (K4, pola T03: audit di luar tx).

Aturan bisnis tetap di entity (`base.Invitation.BlocksLogin`/`IsExpired`); usecase hanya memanggil dan mengorkestrasi; repository satu-satunya yang menyentuh SQL (lewat `base/store` karena `FindInvitation` dipakai lebih dari satu feature, rule 5.3).

Tidak dipecah artifisial: dua usecase yang ada tetap satu usecase masing-masing.

---

## 3. Plan (langkah berurutan)

Kerjakan test-first per langkah. Setiap langkah harus lolos `go build ./...` dan `go test ./internal/domain/core/...`.

### Langkah 1. Login: Repository + usecase

File: `backend/internal/domain/core/internal/auth/login/repository.go`

```go
// FindInvitation mendelegasikan ke store bersama. found=false bila akun tidak punya baris undangan.
func (r *Repo) FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
	return r.store.FindInvitation(ctx, userID)
}
```

File: `backend/internal/domain/core/internal/auth/login/usecase.go`

1. Tambah ke interface `Repository`:
   ```go
   FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
   ```
2. Ubah blok keputusan setelah `Verify`. Semantik yang wajib (kode ilustrasi, bentuk akhir boleh disesuaikan selama perilakunya sama):
   ```go
   reason := "invalid_credentials"
   allowed := found && ok && user.CanLogin()
   if found && ok && !user.CanLogin() {
       reason = "inactive"
   }
   if allowed {
       inv, hasInv, err := u.repo.FindInvitation(ctx, user.ID)
       if err != nil {
           return Result{}, fmt.Errorf("login: cari undangan: %w", err) // fail closed: 500, bukan lolos
       }
       if hasInv && inv.BlocksLogin(now) {
           allowed = false
           reason = "invitation_expired"
       }
   }
   if !allowed {
       // tak berubah: UpdateThrottle RegisterFailure, audit failedEvent(email, up, reason, meta), return base.ErrInvalidCreds
   }
   ```
   - `now` adalah `u.clock()` yang sudah diambil di awal `Execute`; pakai variabel yang sama (satu sumber waktu).
   - Urutan prioritas reason: `inactive` mengalahkan `invitation_expired` (akun nonaktif tidak pernah sampai ke query undangan).
   - Throttle, `failedEvent`, dan `base.ErrInvalidCreds` dipakai apa adanya. Jangan membuat error, kode, pesan, atau header baru. Respons harus byte-identik dengan password salah selain `trace_id`.
   - Jangan menambahkan field apa pun ke `LoginResponse`.
3. `handler.go`, `params.go`, `init.go`: tidak berubah (repo sudah dibuat dengan `NewRepository(d.DB)` dan store ikut).
4. Audit `auth.login.failed` memakai `Args{"email_attempted","reason"}` seperti sekarang; hanya nilai `reason` baru `invitation_expired`. Status tetap `record.StatusDenied`.

### Langkah 2. Changepassword: Repository

File: `backend/internal/domain/core/internal/auth/changepassword/repository.go`

1. Tambah method:
   ```go
   // FindInvitation membungkus store.FindInvitation.
   func (r *Repo) FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
       return r.store.FindInvitation(ctx, userID)
   }
   ```
2. Ubah signature `Change`:
   ```go
   // Change mengganti hash, mencabut sesi lain, dan menandai undangan diterima dalam satu transaksi.
   // revoked: jumlah sesi dicabut. accepted: true bila baris user_invitations berubah dari menunggu menjadi diterima.
   func (r *Repo) Change(ctx context.Context, in ChangeInput) (revoked int64, accepted bool, err error)
   ```
3. Di dalam `BeginTx`, setelah `DeleteSessionsByUser` sukses:
   ```go
   accepted, err = r.store.WithTx(tx).AcceptInvitation(ctx, in.UserID, in.Now)
   if err != nil { return fmt.Errorf("terima undangan: %w", err) }
   ```
   SQL ada di `store.AcceptInvitation` (foundation): `UPDATE user_invitations SET accepted_at=$2 WHERE user_id=$1::uuid AND accepted_at IS NULL`; `accepted = RowsAffected() > 0`.
4. Pada error (termasuk `ErrStale`), kembalikan `0, false, ...` dan pertahankan pemetaan `ErrStale` yang sekarang. Karena `accepted` ditulis dalam closure tx, reset ke `false` bila `BeginTx` mengembalikan error (jangan bocorkan nilai parsial).
5. Tidak ada guard `expires_at` di SQL: kedaluwarsa sudah dicek di usecase; selisih waktu antara cek dan commit dapat diabaikan (user terautentikasi dan password lama terbukti benar). Dicatat sebagai keputusan sadar.

### Langkah 3. Changepassword: usecase

File: `backend/internal/domain/core/internal/auth/changepassword/usecase.go`

1. Interface `Repository`:
   ```go
   FindUserByID(ctx context.Context, id string) (base.User, bool, error)
   FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
   Change(ctx context.Context, in ChangeInput) (revoked int64, accepted bool, err error)
   ```
2. Di `Execute`, ambil `now := u.clock()` sekali di awal (setelah cek identitas), pakai juga untuk `ChangeInput.Now`.
3. Setelah blok `!found || !user.CanLogin()` dan sebelum validasi kebijakan:
   ```go
   inv, hasInv, err := u.repo.FindInvitation(ctx, user.ID)
   if err != nil { return fmt.Errorf("changepassword: cari undangan: %w", err) }
   if hasInv && inv.IsExpired(now) {
       return base.InvalidFields(base.FieldErrors{"current_password": msgInvitationExpired})
   }
   ```
   - Konstanta di `usecase.go`: `msgInvitationExpired = "password sementara sudah kedaluwarsa, minta pengundang mengirim ulang undangan"`.
   - Sengaja tanpa memverifikasi hash dan tanpa mengumpulkan field error lain (satu field, jelas). Pesan boleh spesifik karena pemanggil sudah terautentikasi (tidak membocorkan keadaan akun ke anonim, beda dengan login).
   - Undangan diterima (`AcceptedAt != nil`) tidak pernah expired (`IsExpired` mensyaratkan `IsPending`), jadi akun lama tidak terpengaruh.
4. Panggilan `Change` menjadi `revoked, accepted, err := u.repo.Change(...)`. Pemetaan `ErrStale` -> 409 tetap.
5. Setelah commit:
   ```go
   u.audit.Record(ctx, audit.Event{Action: "auth.password.changed", ...}) // tak berubah
   if accepted {
       u.audit.Record(ctx, audit.Event{
           Action: record.ActionUserInvitationAccepted, TargetType: "user", Target: user.ID,
           Args:   map[string]any{"email": user.Email},
           Status: record.StatusOK, Meta: in.Meta,
       })
   }
   ```
   - Args hanya `email`. Tidak boleh memuat password, hash, atau credential.
   - Import `grimoire/lib/contracts/core/audit/record` di changepassword (pola yang sama dengan `login`).
   - Aktor diturunkan dari ctx identity seperti `auth.password.changed` (tidak diisi manual).
6. `handler.go`, `params.go`, `init.go`: tidak berubah. Respons sukses tetap `204`, dan respons tidak membedakan apakah undangan diterima.

### Langkah 4. Perbarui fake dan tes T01 yang ada

Perubahan signature memaksa fake diperbarui. Ini murni mekanis dan tidak boleh mengubah asersi T01:

- `login/usecase_test.go`: `fakeRepo` tambah field `inv base.Invitation`, `hasInv bool`, `errInv error`, `invFinds int` dan method `FindInvitation`.
- `changepassword/usecase_test.go`: `fakeRepo` tambah `inv`, `hasInv`, `errInv`, `accepted bool`, `invFinds` dan method `FindInvitation`; `Change` mengembalikan `(f.revoked, f.accepted, f.err)`.
- Semua tes T01 lama dijalankan tanpa perubahan asersi (regresi wajib hijau).

### Langkah 5. Dokumen kontrak

- `api/modules/core/auth.yml`: perluas `description` operasi `login` (kalimat: "Akun undangan (T12) yang menunggu dan sudah lewat `expires_at` juga dijawab identik 401 INVALID_CREDENTIALS dan dihitung ke lockout.") dan `changePassword` (kalimat: "Akun undangan yang sudah lewat `expires_at` dijawab 400 dengan `fields.current_password`. Sukses pada akun undangan menandai undangan diterima."). Tidak ada perubahan skema. Jangan menyentuh path lain di file itu (dikerjakan plan usecase lain).
- `make api-bundle` dijalankan pada langkah penutup overview (bagian 9 butir 6), bukan di plan ini, supaya `api/openapi.yml` tidak konflik antar usecase paralel. Bila dikerjakan sendirian, jalankan `make api-bundle` dan commit hasilnya.
- Katalog audit di `docs/tickets/README.md` ditangani foundation; plan ini tidak menyentuhnya.

### Langkah 6. Tes (lihat bagian 7) dan verifikasi akhir

`go build ./...`, `go vet ./...`, `go test ./internal/domain/core/...`, `go test -tags integration ./internal/domain/core/internal/auth/...`, `internal/archtest`, lalu e2e (bagian 8).

---

## 4. Migration

Tidak ada. Tabel `user_invitations` dibuat oleh foundation (`backend/migrations/core/<VERSION>_create_user_invitations.sql`, overview bagian 5). Plan ini hanya membaca dan meng-update kolom `accepted_at`.

---

## 5. Changes (per file)

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/auth/login/repository.go` | ubah | `FindInvitation` (delegasi store) |
| `backend/internal/domain/core/internal/auth/login/usecase.go` | ubah | interface + cek `BlocksLogin` setelah `CanLogin`; reason `invitation_expired` |
| `backend/internal/domain/core/internal/auth/login/usecase_test.go` | ubah | fake + tes baru (bagian 7) |
| `backend/internal/domain/core/internal/auth/login/handler_test.go` | ubah | tes respons identik |
| `backend/internal/domain/core/internal/auth/login/repository_integration_test.go` | ubah | tes `FindInvitation` |
| `backend/internal/domain/core/internal/auth/changepassword/repository.go` | ubah | `FindInvitation`; `Change` mengembalikan `accepted` dan memanggil `AcceptInvitation` dalam tx |
| `backend/internal/domain/core/internal/auth/changepassword/usecase.go` | ubah | tolak kedaluwarsa, audit `user.invitation.accepted` |
| `backend/internal/domain/core/internal/auth/changepassword/usecase_test.go` | ubah | fake + tes baru |
| `backend/internal/domain/core/internal/auth/changepassword/handler_test.go` | ubah | tes 400 `current_password` kedaluwarsa (bila handler test memakai fixture usecase) |
| `backend/internal/domain/core/internal/auth/changepassword/repository_integration_test.go` | ubah | tes accepted (atomik, sekali) |
| `api/modules/core/auth.yml` | ubah | deskripsi `login` dan `changePassword` saja |
| `frontend/e2e/helpers/invitations.ts` | baru (atau pakai ulang bila sudah dibuat plan lain) | helper `expireInvitationDb(email)` |
| `frontend/e2e/invitation-login-accept.spec.ts` | baru | e2e kriteria 2 dan 3 |

Tidak berubah: `handler.go`, `params.go`, `init.go` kedua feature, `base/*` (sudah dari foundation), kontrak bus, wiring `auth/base.go`.

---

## 6. Kontrak bus

Tidak ada endpoint bus baru, tidak ada perubahan kontrak. `core.audit.record` dipanggil lewat `base/audit` (pola yang ada) dengan aksi `user.invitation.accepted`. Tidak ada panggilan bus di dalam tx (audit setelah commit).

---

## 7. Test Scenario

### 7.1 Unit `login/usecase_test.go` (fake repo, hasher argon2 parameter kecil seperti tes yang ada)

| Tes | Skenario | Ekspektasi |
|---|---|---|
| `TestLogin_ExpiredInvitation_SameAsWrongPassword` | user aktif, password benar, undangan menunggu `ExpiresAt = testNow` (tepat batas) | `base.ErrInvalidCreds` (sama pointer/`errors.Is`), tidak ada sesi (`completed` kosong), `updates == 1` (throttle ter-register), satu audit `auth.login.failed` dengan `reason="invitation_expired"` dan `Target=user.ID` |
| `TestLogin_ExpiredInvitation_CountsTowardLockout` | ulangi login kedaluwarsa sebanyak `MaxFailures` | attempt terakhir menghasilkan lock; attempt berikutnya `AccountLocked` dengan audit reason `locked`, identik dengan urutan password salah (bandingkan dengan tes yang sama memakai password salah: urutan kode error dan `LockedUntil` sama) |
| `TestLogin_ExpiredInvitation_ErrorEqualsWrongPassword` | bandingkan `err` hasil kedaluwarsa dengan hasil password salah | `errors.Is` sama, `Code`, `Message`, `Details` sama |
| `TestLogin_PendingNotExpired_Succeeds` | undangan menunggu, `ExpiresAt = testNow + 1ns` | sukses, sesi dibuat, `User.MustChangePassword == true`, audit `auth.login.success`, tidak ada audit failed |
| `TestLogin_AcceptedInvitation_Succeeds` | baris ada, `AcceptedAt` terisi, `ExpiresAt` lampau | sukses (diterima tidak pernah memblokir) |
| `TestLogin_NoInvitation_Unchanged` | `hasInv=false` | sukses persis seperti T01 |
| `TestLogin_ExpiredButWrongPassword_NoInvitationLookup` | password salah untuk akun kedaluwarsa | reason `invalid_credentials`, `invFinds == 0` |
| `TestLogin_UnknownEmail_NoInvitationLookup` | email tidak ada | `invFinds == 0`, dummy hash dipakai (asersi T01 tetap) |
| `TestLogin_InactiveAndExpired_ReasonInactive` | akun nonaktif + kedaluwarsa + password benar | reason `inactive`, `invFinds == 0` |
| `TestLogin_FindInvitationError_FailsClosed` | `errInv` diset | error terbungkus (bukan `ErrInvalidCreds`, bukan sukses), tidak ada sesi, throttle tidak ter-register |
| `TestLogin_ExpiredAudit_NoSecrets` | JSON audit event kedaluwarsa | tidak memuat password maupun hash |

### 7.2 Unit `login/handler_test.go`

- `TestLoginHandler_ExpiredInvitation_IdenticalToWrongPassword`: status 401, tanpa cookie, tanpa `Retry-After`, body sama dengan kasus password salah setelah `trace_id` dihapus, header `Content-Type` sama.

### 7.3 Unit `changepassword/usecase_test.go`

| Tes | Skenario | Ekspektasi |
|---|---|---|
| `TestChangePassword_AcceptsPendingInvitation` | `hasInv=true`, belum expired, `accepted=true` dari fake | sukses; audit berurutan `auth.password.changed` lalu `user.invitation.accepted`; audit kedua `Target=user.ID`, `Args == {"email": ...}`, `Status ok` |
| `TestChangePassword_AcceptedNotRecordedTwice` | panggilan kedua dengan `accepted=false` dari fake | hanya `auth.password.changed`; tidak ada `user.invitation.accepted` |
| `TestChangePassword_NoInvitation_Unchanged` | `hasInv=false`, `accepted=false` | perilaku T01 persis (tes T01 lama lolos tanpa perubahan asersi); satu audit |
| `TestChangePassword_ExpiredInvitationRejected` | undangan menunggu `ExpiresAt = now` | `base.FieldErrors` `current_password` berisi `msgInvitationExpired`; `repo.calls == 0`, `h.verify == 0`, `h.hash == 0`; tidak ada audit |
| `TestChangePassword_JustBeforeExpiryAccepted` | `ExpiresAt = now + 1ns` | sukses |
| `TestChangePassword_AcceptedInvitationNeverExpired` | `AcceptedAt` terisi, `ExpiresAt` lampau | sukses (tidak ditolak) |
| `TestChangePassword_StaleNoAcceptedAudit` | `Change` mengembalikan `ErrStale` | 409 `CONFLICT`; tidak ada audit sama sekali |
| `TestChangePassword_RepoErrorNoAcceptedAudit` | `Change` error lain | error terbungkus; tidak ada audit |
| `TestChangePassword_FindInvitationError` | `errInv` | error terbungkus; `repo.calls == 0` |
| `TestChangePassword_AcceptedAuditNoSecrets` | JSON audit `user.invitation.accepted` | tidak memuat password lama/baru, hash |
| `TestChangePassword_UsesSingleNow` | `ChangeInput.Now` sama dengan waktu yang dipakai cek kedaluwarsa | clock fake dipanggil konsisten |

### 7.4 Unit `changepassword/handler_test.go`

- Kedaluwarsa -> 400 `INVALID_ARGUMENT`, `details.fields.current_password` berisi pesan kedaluwarsa. Sukses undangan -> 204 tanpa body.

### 7.5 Integrasi (`-tags integration`, `db.TestDB`, `db.Migrate` yang ada)

`login/repository_integration_test.go`:
- `TestRepo_FindInvitation`: tanpa baris -> `found=false`; baris menunggu -> semua kolom round-trip; id bukan uuid -> `found=false` tanpa error.

`changepassword/repository_integration_test.go` (seed baris undangan dengan SQL mentah di helper tes):
- `TestChangeRepo_Accept_MarksAcceptedAt`: user + undangan menunggu -> `Change` mengembalikan `accepted=true`, `accepted_at == in.Now` (presisi mikrodetik), `password_hash` dan `must_change_password=false` ikut berubah, sesi lain terhapus.
- `TestChangeRepo_Accept_OnlyOnce`: `Change` kedua (hash baru -> hash lain) mengembalikan `accepted=false`, `accepted_at` tidak berubah dari nilai pertama.
- `TestChangeRepo_NoInvitationRow`: tanpa baris -> `accepted=false`, tidak error, tidak ada baris dibuat.
- `TestChangeRepo_Stale_RollsBackAccept`: `OldHash` salah -> `ErrStale`, `accepted=false`, `accepted_at` tetap NULL, hash tetap.
- `TestChangeRepo_AlreadyAccepted`: baris dengan `accepted_at` terisi -> `accepted=false`, nilai lama tetap.
- `TestChangeRepo_ConcurrentChange_AcceptedExactlyOnce`: dua goroutine `Change` dengan `OldHash` sama -> satu sukses `accepted=true`, satu `ErrStale`; `accepted_at` terisi sekali.
- Tes T01 yang ada (`TestChangeRepo_UpdatesHashAndFlag`, dst.) disesuaikan hanya pada penerimaan nilai kembali ketiga; asersinya tetap.

Regresi wajib: seluruh tes T01 yang ada di `login` dan `changepassword` (unit + integrasi) tetap hijau, `internal/archtest` hijau (changepassword/login hanya meng-import `base`, `base/store`, `base/audit`; tidak ada import antar feature).

---

## 8. E2E Scenarios (Playwright terhadap binary, `frontend/e2e/invitation-login-accept.spec.ts`)

Prasyarat: `auth-inviteuser` (gelombang B) sudah tersedia untuk membuat akun undangan lewat `POST /api/users/invite`; server e2e tanpa `MAIL_DRIVER` (noop) sehingga respons membawa `credentials.temporary_password` (fallback K8), tidak perlu Mailpit. Mengundang dilakukan dari `adminContext`. Kedaluwarsa dibuat lewat helper `expireInvitationDb(email)` di `helpers/invitations.ts`: `spawnSync('psql', [process.env.DATABASE_URL, '-c', "UPDATE user_invitations SET expires_at = now() - interval '1 hour' WHERE user_id = (SELECT id FROM users WHERE email = '<email>')"])`. Email dibatasi `uniqueEmail()` (aman dari injeksi karena dibuat tes sendiri). `INVITATION_TTL` minimum 1 jam sehingga kedaluwarsa tidak bisa dicapai lewat config; helper DB wajib. Tes ditandai skip dengan pesan jelas bila `DATABASE_URL` atau `psql` tidak tersedia.

Positif:
1. (Kriteria 2) Admin mengundang email baru (respons `201` + `credentials`). Penerima `apiLogin` dengan password sementara -> `must_change_password=true`. `GET /api/users` dengan sesi itu -> 403 `PASSWORD_CHANGE_REQUIRED`. Penerima `POST /api/auth/change-password` -> 204. `GET /api/auth/me` -> `must_change_password=false`. Admin `GET /api/users/{id}`: tidak ada lagi penanda undangan menunggu (`invitation` absen/null; bila `auth-invitation-markers` belum merged, verifikasi `accepted_at IS NOT NULL` lewat psql). Audit log (pola `audit-log.spec.ts`) memuat tepat satu `user.invitation.accepted` dengan target user itu dan tanpa password di args.
2. Login UI penuh: penerima membuka `/login`, masuk dengan password sementara, diarahkan ke `/change-password`, mengganti password, masuk ke beranda (alur T01 tidak berubah).
3. (Kriteria 3) Undangan diperbarui kedaluwarsa lewat helper. `POST /api/auth/login` dengan password sementara yang benar -> 401 `INVALID_CREDENTIALS`, body sama (selain `trace_id`) dengan login password salah untuk akun yang sama, tanpa cookie `grimoire_session`. Admin `POST /api/users/{id}/resend-invitation` (gelombang B) -> 200 dengan password baru; login password baru sukses, login password lama 401.
4. Akun tanpa undangan (`createReadyUserApi`) login dan ganti password persis seperti sebelumnya; audit tidak memuat `user.invitation.accepted`.
5. Akun undangan yang sudah diterima (setelah skenario 1, kolom `expires_at` di-set lampau) tetap bisa login dengan password barunya.

Negatif:
1. Akun kedaluwarsa + password sementara benar, diulang 5 kali (`LOGIN_RATE_PER_MINUTE=1000` di e2e sehingga rate limit IP tidak ikut campur) -> percobaan ke-6 `429 ACCOUNT_LOCKED` dengan `Retry-After`, sama dengan perilaku password salah.
2. Sesi dibuat saat undangan masih berlaku, lalu undangan dibuat kedaluwarsa. `POST /api/auth/change-password` dengan sesi itu -> 400 `INVALID_ARGUMENT` `details.fields.current_password`; `must_change_password` tetap true; `accepted_at` tetap NULL; setelah kirim ulang, sesi lama sudah dicabut (401), login password baru -> ganti password sukses.
3. Akun kedaluwarsa + password salah -> 401 yang sama dengan skenario kedaluwarsa + password benar (tidak ada pembeda).
4. Akun nonaktif dengan undangan kedaluwarsa -> 401 `INVALID_CREDENTIALS` (tidak ada pembeda).
5. Penerima menjalankan change-password dua kali: kali kedua (password lama = password baru pertama) tidak menghasilkan audit `user.invitation.accepted` kedua (tetap satu).
6. Password baru lemah pada akun undangan menunggu -> 400 per field dan `accepted_at` tetap NULL (tidak ada penerimaan setengah jadi).

---

## 9. Request/Response body

Semua mutasi memerlukan header `X-Requested-With: grimoire`. Envelope error mengikuti `api/modules/infra/foundation.yml` (`error.code`, `error.message`, `error.details`, `trace_id` di level atas).

### 9.1 `POST /api/auth/login`

Request (sama untuk semua kasus):
```json
{ "email": "baru@contoh.id", "password": "q7XkR2mVtz9HbNcP" }
```

200, undangan menunggu dan belum kedaluwarsa (set-cookie `grimoire_session`, `Cache-Control: no-store`; bentuk tidak berubah):
```json
{
  "user": {
    "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "email": "baru@contoh.id",
    "name": "baru",
    "role": "developer",
    "must_change_password": true
  }
}
```

401, undangan kedaluwarsa (BYTE-IDENTIK dengan password salah/email tidak ada, selain `trace_id`; tanpa cookie, tanpa `Retry-After`):
```json
{
  "error": { "code": "INVALID_CREDENTIALS", "message": "email atau password salah" },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

429, setelah ambang gagal tercapai (header `Retry-After: 812`), sama untuk kedaluwarsa dan password salah:
```json
{
  "error": {
    "code": "ACCOUNT_LOCKED",
    "message": "terlalu banyak percobaan login, coba lagi nanti",
    "details": { "retry_after_seconds": 812 }
  },
  "trace_id": "9b2d7e10-3c4a-4f68-8a21-5d6e7f809a1b"
}
```

400, validasi (tak berubah):
```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "email": "wajib diisi", "password": "wajib diisi" } } },
  "trace_id": "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9"
}
```

### 9.2 `POST /api/auth/change-password`

Request:
```json
{ "current_password": "q7XkR2mVtz9HbNcP", "new_password": "Rahasia-Panjang-123!" }
```

204 sukses (tanpa body; sama untuk akun undangan dan akun biasa; undangan ditandai diterima di sisi server tanpa tanda di respons).

400, undangan kedaluwarsa:
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": {
      "fields": {
        "current_password": "password sementara sudah kedaluwarsa, minta pengundang mengirim ulang undangan"
      }
    }
  },
  "trace_id": "6c5b4a39-2817-4f06-9e5d-4c3b2a190807"
}
```

400, password lama salah atau password baru lemah (tak berubah):
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "current_password": "password lama salah", "new_password": "minimal 10 karakter" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

409, perubahan password paralel (tak berubah):
```json
{
  "error": { "code": "CONFLICT", "message": "password berubah oleh proses lain, muat ulang dan coba lagi" },
  "trace_id": "0f1e2d3c-4b5a-4968-8776-6554433221100"
}
```

401, sesi tidak valid (tak berubah):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "a1b2c3d4-e5f6-4789-8abc-def012345678" }
```

### 9.3 Audit (via `core.audit.record`, bukan respons HTTP)

`auth.login.failed` untuk kedaluwarsa:
```json
{
  "action": "auth.login.failed", "status": "denied", "target_type": "user", "target": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "args": { "email_attempted": "baru@contoh.id", "reason": "invitation_expired" }
}
```

`user.invitation.accepted` setelah `change-password` sukses:
```json
{
  "action": "user.invitation.accepted", "status": "ok", "target_type": "user", "target": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "args": { "email": "baru@contoh.id" }
}
```

---

## 10. Risiko dan catatan

- Jalur kritis auth: seluruh tes T01 yang ada wajib hijau. Perubahan kode di `login` hanya menambah satu query setelah password benar dan akun aktif.
- Kebocoran lewat waktu respons: akun kedaluwarsa melakukan satu query tambahan dibanding password salah. Selisihnya jauh di bawah biaya verifikasi argon2 dan tidak dimitigasi lebih lanjut; respons, status, dan hitungan lockout identik (K3). Pembeda hanya di audit internal.
- `FindInvitation` gagal -> fail closed (500), tidak pernah meloloskan login.
- Race resend vs change-password: resend mengubah `password_hash` dan mencabut semua sesi, sehingga `Change` yang berjalan bersamaan gagal dengan `ErrStale` (409) lewat guard hash lama yang sudah ada. Tidak perlu lock tambahan.
- Pesan kedaluwarsa di changepassword boleh spesifik karena hanya dapat dicapai oleh sesi terautentikasi; login tetap tidak membocorkan.
- Ketergantungan K3 (masa berlaku 7 hari) dan K4 masih menunggu persetujuan pemilik (tiket); plan mengikutinya apa adanya.
