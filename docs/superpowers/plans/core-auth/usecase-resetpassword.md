# T01 core/auth — Usecase `resetpassword`

Plan rinci satu usecase. Wajib mengikuti `docs/superpowers/plans/core-auth/overview.md` (selanjutnya "overview"); bila bertentangan, overview menang sampai diperbarui. Sumber aturan: `rules.txt`, `docs/tickets/README.md`, `docs/tickets/T01-core-auth.md`.

Konteks: belum ada `CLAUDE.md`, `.claude/rules/`, maupun knowledge doc module auth. Plan ini tidak menambah migrasi, kontrak bus, konfigurasi, atau middleware; semuanya sudah disediakan F1 (overview bagian 8).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (plan ini hanya backend; UI dialog reset di `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Usecase / folder | `resetpassword` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Endpoint | `/api/users/{id}/reset-password` |
| Method | POST |
| Version | tanpa prefix versi di URL (`api/modules/core/auth.yml` `info.version: 0.1.0`) |
| Auth | `RequireAuth` + `RequireRole(super_admin)` |
| Password gate | dicek (admin dengan `must_change_password=true` harus ganti password dulu) |
| Gelombang | C (setelah F1, `login`, `changepassword`) |
| Endpoint bus | tidak menyediakan. Mengonsumsi `core.audit.record` (Call, via `base/audit`) |
| Job / CLI | tidak ada |
| Bergantung pada | F1 (config, `base/*`, `base/store`, `base/audit`, migrasi, `auth.yml`) |

---

## 2. Summary Plan

Super Admin menetapkan password sementara baru untuk user lain. Alur:

1. Handler membaca `{id}` dari path dan body opsional `{ "password": "..." }` (body kosong atau `{}` sah), lalu meta request.
2. Usecase memuat target (404 bila tidak ada atau `id` bukan uuid), menolak reset akun sendiri, lalu menentukan password: bila `password` kosong dibangkitkan `GeneratePassword`; bila diisi divalidasi `ValidatePassword` terhadap email target.
3. Password di-hash argon2id DI LUAR transaksi.
4. Repository dalam SATU transaksi: `UPDATE users SET password_hash, must_change_password=true, updated_at ... RETURNING` lalu `DELETE FROM sessions WHERE user_id=$1` (semua sesi target).
5. Setelah commit: audit `auth.password.reset` (fail-open, tanpa password), lalu respons 200 `{user, temporary_password}` dengan `Cache-Control: no-store`. `temporary_password` terisi hanya bila dibangkitkan server (SEKALI, tidak bisa diambil lagi); bila admin mengisi sendiri nilainya `null`.

Keputusan desain khusus usecase ini:

| # | Keputusan | Alasan |
|---|---|---|
| R1 | Reset akun sendiri ditolak 403 `FORBIDDEN` dengan `details.reason = "SELF_ACTION_FORBIDDEN"` dan pesan `"gunakan ganti password untuk akun sendiri"` | Reset menghapus semua sesi target, termasuk sesi admin sendiri. Admin akan terlempar keluar dan password sementara hanya ada di respons yang mungkin hilang (risiko terkunci keluar). Jalur yang benar untuk akun sendiri adalah `changepassword`. Tiket tidak menyebut kasus ini; keputusan dicatat di PR. Pesan lokal feature (tidak memakai `base.ErrSelfAction` karena kalimatnya khusus deactivate/role) |
| R2 | Target nonaktif boleh direset | Admin boleh menyiapkan akun sebelum diaktifkan lagi. Sesi sudah kosong, DELETE no-op |
| R3 | Hash argon2 dihitung di luar transaksi (pola C3 changepassword) | Tidak menahan koneksi dan row lock selama puluhan ke ratusan ms |
| R4 | Tanpa guard optimistik hash lama. `UPDATE ... WHERE id=$1` saja | Reset adalah penimpaan oleh admin; yang terakhir menang. Race dengan `changepassword` target: bila reset commit lebih dulu, `changepassword` kena guard hash lama miliknya dan mendapat 409 (benar). Bila `changepassword` commit lebih dulu, reset menimpa dan sesi dihapus (benar) |
| R5 | Lockout login (`login_throttles`) target TIDAK dibersihkan oleh reset | Di luar spesifikasi tiket dan overview 3.8 (hanya tabel `sessions`). Kunci kedaluwarsa sendiri; menyentuh tabel lain menambah permukaan. Dicatat sebagai non-goal |
| R6 | `Cache-Control: no-store` dipasang pada setiap respons sukses | Seragam dengan `createuser`; respons membawa kredensial |
| R7 | Body opsional ditangani handler (EOF = body kosong), bukan 400 | Klien FE memanggil tombol reset tanpa form; `{}` dan tanpa body sama-sama berarti "bangkitkan" |
| R8 | Target `id` bukan uuid menghasilkan 404 `NOT_FOUND` `"user tidak ditemukan"` | Sesuai overview 3.12 |
| R9 | Hasil reset TIDAK memuat flag apa pun selain `UserDTO` terbaru; `must_change_password` selalu `true` | Entity `UserDTO` sudah memuatnya |

---

## 3. Plan (langkah berurutan)

Aturan umum: TDD per langkah (test dulu). Tidak mengubah signature publik T00. Nama simbol T00 sesuai overview bagian 0 (`httpx`). Feature ini hanya meng-import `base`, `base/store`, `base/audit`, `lib/*`, `infra/{app,db,<httpx>}`; TIDAK meng-import feature lain (rule 4.3), termasuk `createuser`.

### Langkah 0 — Prasyarat dari F1 (verifikasi, jangan duplikasi)

Pastikan sudah ada: `base.Shared`, `base.Secret`, `base.Hasher`, `base.PasswordPolicy.ValidateField`, `base.GeneratePassword`, `base.User`/`UserDTO`, `base.ErrUserNotFound`, `base.InvalidFields`, `base.RequestMeta`, `base.Clock`, `store.FindUserByID`, `store.DeleteSessionsByUser`, `audit.Recorder`/`audit.Event`, tabel `users` dan `sessions`, serta operasi OpenAPI `resetPassword`. Yang belum ada, tambahkan di PR ini dengan test.

Tambahan aditif di `base/` bila belum ada (murni, tanpa I/O):

```go
// base/user.go
// ResetPasswordState mengembalikan salinan user dengan hash baru dan flag wajib ganti aktif.
// Dipakai test entity; SQL repository mencerminkan aturan yang sama.
func (u User) WithResetPassword(hash string, now time.Time) User // PasswordHash=hash, MustChangePassword=true, UpdatedAt=now
```

### Langkah 1 — OpenAPI dulu (rule 11.6)

Verifikasi `api/modules/core/auth.yml` memuat operasi `POST /users/{id}/reset-password` (`operationId: resetPassword`):
- parameter path `id` (uuid), header wajib `X-Requested-With`, `security: sessionCookie`;
- request `ResetPasswordRequest {password?: string}` (`required: false` pada requestBody);
- respons 200 `UserWithTempPassword` dengan header `Cache-Control: no-store`; 400, 401, 403 (contoh `FORBIDDEN`, `PASSWORD_CHANGE_REQUIRED`, `SELF_ACTION_FORBIDDEN`), 404, 429, 500.

Jalankan `make api-bundle`, commit `api/openapi.yml`, pastikan `make api-check` lulus.

### Langkah 2 — Folder feature `resetpassword/`

Path: `backend/internal/domain/core/internal/auth/resetpassword/`. Satu usecase, satu folder; tidak dipecah.

**`params.go`**

```go
package resetpassword

type request struct {
    Password base.Secret `json:"password"` // opsional; "" atau tidak dikirim = bangkitkan
}

type Input struct {
    TargetID string
    Password base.Secret
    Meta     base.RequestMeta
}

// Result hanya keluar lewat WriteJSON di handler. TIDAK dilog, TIDAK masuk audit.Event.
type Result struct {
    User              base.UserDTO `json:"user"`
    TemporaryPassword *string      `json:"temporary_password"` // tanpa omitempty: null bila admin yang mengisi
}
```

**`repository.go`** (satu-satunya yang menyentuh SQL)

```go
type Repo struct { db db.DB; store *store.Store }
func NewRepo(d db.DB) *Repo

// FindUserByID membungkus store.FindUserByID (id bukan uuid -> found=false tanpa query).
func (r *Repo) FindUserByID(ctx context.Context, id string) (base.User, bool, error)

type ResetInput struct {
    UserID string
    Hash   string
    Now    time.Time
}
var ErrTargetGone = errors.New("resetpassword: user tidak ada")

// Reset menjalankan dalam SATU transaksi (BeginTx dipanggil di sini, bukan di usecase):
//  1. UPDATE users SET password_hash=$2, must_change_password=true, updated_at=$3
//       WHERE id=$1
//       RETURNING <kolom user eksplisit>        -> tidak ada baris => rollback, ErrTargetGone
//  2. DELETE FROM sessions WHERE user_id=$1     -> revoked = RowsAffected (via store.WithTx(tx).DeleteSessionsByUser(ctx, id, nil))
// Mengembalikan user terbaru dan jumlah sesi dicabut. Commit hanya bila kedua langkah sukses.
func (r *Repo) Reset(ctx context.Context, in ResetInput) (base.User, int64, error)
```

Catatan SQL: semua query berparameter, kolom eksplisit (tanpa `SELECT *`). `UPDATE` sudah mengunci baris sehingga tidak perlu `FOR UPDATE` terpisah. Query UPDATE khusus feature ini, jadi ditaruh di `repository.go`, bukan `base/store`. Dipetakan ke `base.User` lewat row struct privat bertag `db` (sama dengan `store`; bila `store` mengekspos helper scan, pakai itu).

**`usecase.go`**

```go
type Repository interface {
    FindUserByID(ctx context.Context, id string) (base.User, bool, error)
    Reset(ctx context.Context, in ResetInput) (base.User, int64, error)
}

type Usecase struct {
    repo   Repository
    hasher base.Hasher
    audit  audit.Recorder
    clock  base.Clock
    rand   io.Reader
    policy base.PasswordPolicy
}
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, clock base.Clock, rnd io.Reader, policy base.PasswordPolicy) *Usecase
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error)

var errSelfReset = errs.WithDetails(errs.CodeForbidden,
    "gunakan ganti password untuk akun sendiri",
    map[string]any{"reason": base.ReasonSelfAction})
```

Urutan `Execute` (orkestrasi saja; rule ada di `base`, rule 5.5):
1. `actor, ok := reqctx.IdentityFrom(ctx)`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")` (pertahanan berlapis; router sudah menjaga).
2. `target, found, err := repo.FindUserByID(ctx, in.TargetID)`. Error -> `fmt.Errorf("resetpassword: cari user: %w", err)`. `!found` -> `base.ErrUserNotFound` (404).
3. `target.ID == actor.UserID` -> `errSelfReset` (R1). Cek ini SEBELUM validasi password dan hashing.
4. Password:
   - `pw := in.Password.Reveal()`; `generated := pw == ""`.
   - Bila `!generated`: `fe := policy.ValidateField("password", pw, target.Email)`; `len(fe) > 0` -> `base.InvalidFields(fe)` (tanpa hashing, tanpa DB write, tanpa audit).
   - Bila `generated`: `pw, err = base.GeneratePassword(u.rand)`; error -> `fmt.Errorf("resetpassword: generate password: %w", err)`.
5. `hash, err := hasher.Hash(pw)`; error -> `fmt.Errorf("resetpassword: hash: %w", err)` (pesan tidak memuat pw).
6. `updated, revoked, err := repo.Reset(ctx, ResetInput{UserID: target.ID, Hash: hash, Now: clock()})`:
   - `errors.Is(err, ErrTargetGone)` -> `base.ErrUserNotFound` (balapan: user hilang di antara langkah 2 dan 6; praktis tidak terjadi karena user tidak pernah dihapus, tetap ditangani);
   - error lain -> `fmt.Errorf("resetpassword: simpan: %w", err)`.
7. Setelah commit: `audit.Record(ctx, audit.Event{Action: "auth.password.reset", TargetType: "user", Target: target.ID, Args: map[string]any{"generated": generated, "revoked_sessions": revoked}, Status: record.StatusOK, Meta: in.Meta})`. `Actor` nil sehingga dari `reqctx` (actor_type `user`, id dan label = admin). Fail-open: `Record` tidak mengembalikan error.
8. `res := Result{User: updated.ToResponse()}`; bila `generated`, `res.TemporaryPassword = &pw`. Return.

Aturan tambahan: usecase tidak meng-import `net/http`, tidak menyentuh SQL, tidak me-log apa pun yang berasal dari password. Penegakan `super_admin` hanya di router (`RequireRole`); usecase tidak mengulang cek role. Args audit TIDAK memuat password, hash, maupun email target (target sudah ada di kolom `target`).

**`handler.go`**

```go
type Handler struct{ uc *Usecase }

func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
    var req request
    empty, err := bodyIsEmpty(r)         // peek 1 byte lewat bufio; io.EOF = kosong; body dikembalikan utuh
    if err != nil { httpx.WriteError(w, r, err); return }
    if !empty {
        if err := httpx.DecodeJSON(r, &req); err != nil { httpx.WriteError(w, r, err); return }
    }
    res, err := h.uc.Execute(r.Context(), Input{
        TargetID: chi.URLParam(r, "id"),
        Password: req.Password,
        Meta: base.RequestMeta{IP: httpx.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
    })
    if err != nil { httpx.WriteError(w, r, err); return }
    w.Header().Set("Cache-Control", "no-store")
    httpx.WriteJSON(w, http.StatusOK, res)
}
```

- `bodyIsEmpty`: helper privat di handler. Bila `r.Body == http.NoBody` atau `r.ContentLength == 0` -> kosong. Selain itu bungkus `r.Body` dengan `bufio.NewReader`, `Peek(1)`; `io.EOF` -> kosong; jika tidak, ganti `r.Body` dengan reader yang sudah di-peek agar `DecodeJSON` membaca utuh. Verifikasi saat implementasi bagaimana `DecodeJSON` T00 memperlakukan body kosong; bila sudah mentoleransi EOF, helper ini dihapus dan test `TestHandler_EmptyBody` tetap menguncinya.
- `truncate` dipakai juga feature lain; pakai helper bersama di `base` bila F1 sudah punya, jika belum definisikan privat di handler (jangan import feature lain).
- Tidak ada logging di handler. Error 500 disamarkan oleh `WriteError`.

**`init.go`**

```go
func Init(d app.Deps, s base.Shared) {
    repo := NewRepo(d.DB)
    uc := NewUsecase(repo, s.Hasher, audit.New(d.Bus, d.Log), s.Clock, s.Rand, s.Password)
    h := &Handler{uc: uc}
    d.API.With(httpx.RequireAuth, httpx.RequireRole(resolvesession.RoleSuperAdmin)).
        Post("/users/{id}/reset-password", h.Reset)
}
```

- Gunakan `d.API.With(...).Post(...)`, BUKAN `d.API.Route("/users", ...)`: feature lain (`createuser`, `listusers`, `getuser`, `updateuser`, `setuserstatus`) mendaftarkan pola `/users...` dan chi tidak mengizinkan dua `Route`/`Mount` pada pola yang sama.
- Ikuti bentuk pemanggilan `RequireAuth`/`RequireRole` yang dipakai T00 (fungsi atau pabrik); sesuaikan, jangan menebak. Pemasangan `Timeout`, `CSRF`, `Session`, `PasswordChangeGate` sudah di grup `/api`, jangan dipasang ulang.
- Rate limit khusus tidak dipasang (aksi admin, volume rendah).
- `resolvesession.RoleSuperAdmin` dari `lib/contracts/core/auth/resolvesession` (hanya konstanta kontrak, boleh diimpor).

**`auth/base.go`**: tambah SATU baris `resetpassword.Init(d, shared)` (dan importnya). Jangan mengubah baris feature lain.

### Langkah 3 — Verifikasi

`go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, `go test -tags integration` untuk integrasi, `TestT00_AC4_RepoIsClean` (archtest), `make api-check`.

---

## 4. Migration

Tidak ada migrasi baru. Memakai `users` dan `sessions` dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (F1). Kolom yang ditulis: `users.password_hash`, `users.must_change_password`, `users.updated_at`; baris `sessions` dihapus per `user_id` (didukung `sessions_user_id_idx`).

---

## 5. Changes (per file/folder)

Baru, di `backend/internal/domain/core/internal/auth/resetpassword/`:
- `init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go`
- `usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (tag `integration`)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `resetpassword.Init(d, shared)` + import.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go`: baris `resetpassword` pada `TestT01_AC8_PasswordNeverLoggedOrAudited` dan `TestT01_AC9_EachActionOneAuditEntry`; `POST /api/users/{id}/reset-password` pada `TestT01_AC7_DeveloperForbiddenOnUsers` dan `TestRoutesRegistered`; test baru `TestT01_ResetPassword_*` (bagian 6).

Diubah hanya bila belum ada dari F1 (tandai di PR sebagai bagian F1):
- `base/user.go` (+ `user_test.go`): `User.WithResetPassword`.
- `api/modules/core/auth.yml`, `api/openapi.yml`: operasi `resetPassword`.

Kontrak bus: tidak ada yang baru. Event audit memakai `record.Request` (overview 5.5):

```go
record.Request{
    ActorType: "user", ActorID: "<admin id>", ActorLabel: "admin@tim.id",
    Action: "auth.password.reset", TargetType: "user", Target: "<target id>",
    Args: map[string]any{"generated": true, "revoked_sessions": 2},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "<trace id>",
}
```

Tidak diubah: infra T00, feature lain, `docs/tickets/*` (ditangani F1).

---

## 6. Test Scenario

Penamaan test AC mengikuti rule 13.4. Hasher test memakai `Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}`.

### Unit `base/` (tanpa mock)
- `TestUser_WithResetPassword`: hash terganti, `MustChangePassword=true`, `UpdatedAt=now`, field lain tidak berubah, nilai asal tidak dimutasi (value receiver).
- Rule password (`ValidateField`, `GeneratePassword`) sudah diuji F1; tidak diulang.

### Unit usecase (`usecase_test.go`; fake `Repository`, hasher nyata, recorder fake, clock tetap, `Rand` deterministik)

| Test | Skenario | Ekspektasi |
|---|---|---|
| `TestExecute_GeneratesPasswordWhenEmpty` | `Password==""` | `TemporaryPassword` terisi, panjang 16, hasher menerima nilai yang sama, `Args["generated"]==true` |
| `TestExecute_UsesProvidedPassword` | password valid | `TemporaryPassword==nil`, hasher menerima password itu, `Args["generated"]==false` |
| `TestExecute_ResetInputShape` | sukses | `Reset` dipanggil sekali dengan `UserID=target.ID`, `Hash` memverifikasi password, `Now=clock()`; hash bukan plaintext |
| `TestExecute_PasswordTooShort` | 9 rune | 400 `fields.password="minimal 10 karakter"`; hasher, `Reset`, audit tidak dipanggil |
| `TestExecute_PasswordEqualsTargetEmail` | `password == target.Email` beda huruf | `fields.password="password tidak boleh sama dengan email"` (dibandingkan dengan email TARGET, bukan admin) |
| `TestExecute_PasswordTooLong` | 257 byte | `fields.password="maksimal 256 byte"` |
| `TestExecute_TargetNotFound` | repo `found=false` | `NOT_FOUND`; tanpa hashing, tanpa `Reset`, tanpa audit |
| `TestExecute_SelfResetForbidden` | target.ID == actor.UserID | 403 `FORBIDDEN`, `details.reason="SELF_ACTION_FORBIDDEN"`; dicek sebelum validasi password (password invalid tetap memberi 403); tanpa hashing/`Reset`/audit |
| `TestExecute_InactiveTargetAllowed` | target `IsActive=false` | sukses |
| `TestExecute_TargetGoneRace` | `Reset` mengembalikan `ErrTargetGone` | `NOT_FOUND`; audit tidak dipanggil |
| `TestExecute_RepoErrorWrapped` | `Reset` error generik | error terbungkus `resetpassword: simpan: ...`, bukan `*errs.Error` klien, tidak memuat password/hash; audit tidak dipanggil |
| `TestExecute_FindErrorWrapped` | `FindUserByID` error | terbungkus; tanpa efek samping |
| `TestExecute_HasherErrorNoLeak` | hasher gagal | terbungkus tanpa plaintext; `Reset` tidak dipanggil |
| `TestExecute_RandError` | `Rand` gagal | terbungkus; tanpa `Reset` |
| `TestExecute_NoIdentity` | ctx tanpa identity | `UNAUTHORIZED`; tanpa efek samping |
| `TestExecute_AuditEventShape` | sukses | tepat 1 event: `Action=auth.password.reset`, `TargetType=user`, `Target=<id>`, Args hanya `{generated, revoked_sessions}`, `Status=ok`, `Meta` diteruskan, `Actor==nil` |
| `TestExecute_AuditNeverContainsSecrets` | mode generated dan provided | marshal `Event` ke JSON; tidak memuat plaintext maupun hash |
| `TestExecute_ResultHasNoHash` | sukses | marshal `Result` tidak memuat `password_hash` maupun nilai hash; `user.must_change_password==true` |
| `TestExecute_AuditFailureDoesNotFail` | recorder gagal diam-diam | `Execute` tetap sukses (fail-open) |

### Handler (`handler_test.go`, httptest, usecase asli dengan fake repo)
- `TestHandler_Success200`: status 200, `Content-Type` JSON, `Cache-Control: no-store`, body sesuai bagian 8.
- `TestHandler_EmptyBody`: tanpa body, `Content-Length: 0`, dan chunked kosong semuanya memicu mode generate (200, `temporary_password` terisi).
- `TestHandler_EmptyObject`: `{}` sama dengan kosong; `{"password":""}` sama.
- `TestHandler_ProvidedPasswordNull`: body memuat `"temporary_password":null` (kunci ada).
- `TestHandler_InvalidJSON400`, `TestHandler_WrongType400` (`{"password": 5}`).
- `TestHandler_UnknownFieldBehavior`: kunci perilaku `DecodeJSON` T00 untuk field asing (`must_change_password`, `is_active`, `role`); apa pun hasilnya (diabaikan atau 400), user hasil reset tetap `must_change_password=true` dan field itu tidak berpengaruh.
- `TestHandler_BadUUID404`: `{id}` = `abc` -> 404 `NOT_FOUND`.
- `TestHandler_InternalErrorHidesDetail`: 500 generik tanpa teks SQL/pgx/password.
- `TestHandler_SecretNotLoggedOnDecodeError`: body invalid yang memuat `password` tidak muncul di buffer log.
- `TestHandler_NoStoreNotSetOnError`: header `Cache-Control` tidak wajib pada error; error tidak membawa kredensial (cukup assert body tanpa `temporary_password`).

### Integrasi repository (Postgres nyata via `db.TestDB`, migrasi `migrations.FS`)
- `TestResetRepo_UpdatesHashFlagAndTimestamp`: `password_hash` baru, `must_change_password=true`, `updated_at=now`; `RETURNING` mengembalikan user terbaru; kolom lain (email, role, is_active, created_*, last_login_at) tidak berubah.
- `TestResetRepo_DeletesAllTargetSessions`: target punya 3 sesi -> revoked=3 dan 0 tersisa; sesi user lain tidak tersentuh.
- `TestResetRepo_UserNotFound_NoSideEffects`: id uuid acak -> `ErrTargetGone`; tidak ada sesi terhapus.
- `TestResetRepo_Atomic`: sisipkan kegagalan pada langkah DELETE (mis. tx dengan konteks dibatalkan setelah UPDATE melalui hook uji, atau trigger sementara pada `sessions` yang melempar error); hasilnya hash TIDAK berubah dan sesi masih ada (rollback penuh).
- `TestResetRepo_InactiveTarget`: user nonaktif tetap terupdate.
- `TestResetRepo_ConcurrentWithChangePasswordGuard`: reset lebih dulu, lalu UPDATE bergaya changepassword dengan `password_hash=<hash lama>` mengenai 0 baris (membuktikan R4).

### Integrasi lintas module (`auth_integration_test.go`; router lengkap, bus nyata, handler `core.audit.record` palsu bila T02 belum merge)
- `TestT01_ResetPassword_GeneratedFlow`: admin reset developer tanpa body -> 200, `temporary_password` 16 karakter; developer login dengan password lama -> 401 `INVALID_CREDENTIALS`; login dengan password sementara -> 200 dan `must_change_password=true`; `GET /api/users` oleh developer itu -> 403 `PASSWORD_CHANGE_REQUIRED`... (role developer: pakai `GET /api/auth/me` 200 dan route non-exempt apa pun 403 `PASSWORD_CHANGE_REQUIRED` karena gate berjalan sebelum cek role).
- `TestT01_ResetPassword_ProvidedPassword`: admin mengirim password valid -> 200, `temporary_password: null`, login dengan password itu 200.
- `TestT01_ResetPassword_KillsTargetSessions`: developer punya dua sesi aktif (A, B); reset -> request dengan A dan B memberi 401 + `Set-Cookie` `Max-Age=0`; tabel `sessions` target kosong. Sesi ADMIN tetap hidup (request admin berikutnya 200).
- `TestT01_ResetPassword_ClearsMustChangeAfterChange`: setelah login dengan password sementara, `changepassword` 204 menurunkan flag; akses normal.
- `TestT01_ResetPassword_SelfForbidden`: admin reset dirinya -> 403 `SELF_ACTION_FORBIDDEN`; hash dan sesi tidak berubah; 0 entri audit.
- `TestT01_ResetPassword_UnknownAndBadID`: uuid acak -> 404; `abc` -> 404; 0 entri audit.
- `TestT01_ResetPassword_ValidationErrors`: password 9 karakter, sama dengan email target -> 400 dengan `fields.password`; hash di DB tidak berubah; sesi target masih ada; 0 entri audit.
- `TestT01_ResetPassword_InactiveTarget`: target nonaktif -> 200; target tetap nonaktif (`is_active=false`); login tetap 401.
- `TestT01_ResetPassword_RequiresCSRFHeader`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
- `TestT01_ResetPassword_Anonymous401`.
- `TestT01_ResetPassword_FlaggedAdminBlocked`: admin dengan `must_change_password=true` memanggil reset -> 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestT01_AC7_DeveloperForbiddenOnUsers` (tambah baris): developer (flag false) memanggil reset -> 403 `FORBIDDEN`; hash target tidak berubah.
- `TestT01_AC8_PasswordNeverLoggedOrAudited` (tambah baris): mode generated dan provided, termasuk jalur gagal validasi; password sementara, password yang dikirim, token cookie, dan hash PHC tidak muncul di buffer log maupun `record.Request` (Args/Result di-marshal).
- `TestT01_AC9_EachActionOneAuditEntry` (tambah baris): reset sukses menghasilkan tepat satu `record.Request` `auth.password.reset` dengan `Args.generated` dan `Args.revoked_sessions` benar; gagal (400, 403, 404) menghasilkan nol.
- `TestAudit_UnregisteredDoesNotFailRequest` (tambah baris): tanpa handler audit, reset tetap 200 dan log berisi `audit record gagal`.
- `TestRoutesRegistered`: `POST /api/users/{id}/reset-password` ada.

---

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Admin membuka Users, memilih developer, klik Reset password, konfirmasi tanpa mengisi password. Dialog menampilkan password sementara SEKALI dengan tombol salin; tombol salin berfungsi. Menutup dialog lalu membukanya lagi tidak menampilkan password itu lagi.
2. Developer (dengan sesi aktif di browser lain) mendapat 401 pada aksi berikutnya dan diarahkan ke `/login`. Login dengan password sementara berhasil dan otomatis diarahkan ke `/change-password`; setelah ganti password masuk beranda.
3. Admin mereset dengan mengisi password sendiri (>= 10 karakter): dialog menyatakan password diatur, tidak menampilkan password; developer bisa login dengan password itu lalu wajib menggantinya.
4. Reset terhadap user nonaktif berhasil; user tetap tidak bisa login sampai diaktifkan.
5. Daftar Users setelah reset: kolom status/ "wajib ganti password" user target tampil sesuai (`must_change_password=true`).

Negatif:
1. Admin mencoba mereset dirinya: tombol reset disembunyikan atau dinonaktifkan untuk baris diri sendiri; API manual -> 403 `SELF_ACTION_FORBIDDEN`.
2. Password diisi 9 karakter: pesan `minimal 10 karakter` pada field password dialog; tidak ada sesi yang terputus.
3. Password sama dengan email target: pesan `password tidak boleh sama dengan email`.
4. Developer memanggil `POST /api/users/{id}/reset-password` lewat fetch manual: 403 `FORBIDDEN`.
5. Admin ber-flag `must_change_password` memanggil endpoint ini: 403 `PASSWORD_CHANGE_REQUIRED`; UI mengarahkan ke `/change-password`.
6. `{id}` tidak ada atau bukan uuid: 404 `user tidak ditemukan`; UI menampilkan pesan dan menyegarkan daftar.
7. Tanpa header `X-Requested-With`: 403 `CSRF_REQUIRED`. Tanpa cookie: 401.
8. Dua admin mereset user yang sama hampir bersamaan: kedua 200 dengan password berbeda; hanya password milik reset terakhir yang bisa login (yang terakhir commit menang).
9. Respons reset tidak muncul di riwayat cache browser: header `Cache-Control: no-store` ada pada respons.

---

## 8. Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire` dan cookie `grimoire_session` milik super_admin.

### POST /api/users/{id}/reset-password

Request, bangkitkan otomatis (body kosong, `{}`, atau `{"password": ""}`):

```json
{}
```

Request, admin menetapkan sendiri:

```json
{ "password": "sementara-baru-2026" }
```

200 (header `Cache-Control: no-store`), password dibangkitkan:

```json
{
  "user": {
    "id": "7d1e5b3a-0c4f-4a9e-8b21-6f3d9a7c1e55",
    "email": "sari@tim.id",
    "name": "Sari",
    "role": "developer",
    "is_active": true,
    "must_change_password": true,
    "created_at": "2026-10-21T09:00:00Z",
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "admin@tim.id",
    "updated_at": "2026-10-21T10:30:00Z",
    "last_login_at": "2026-10-21T09:10:00Z"
  },
  "temporary_password": "Hn4wTq8ZmP2kRx7A"
}
```

200, password diisi admin (`temporary_password` null, nilai yang dikirim tidak pernah dikembalikan):

```json
{
  "user": { "id": "7d1e5b3a-0c4f-4a9e-8b21-6f3d9a7c1e55", "must_change_password": true, "...": "bentuk UserDTO lengkap" },
  "temporary_password": null
}
```

400 validasi password:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "password": "minimal 10 karakter" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Pesan lain untuk `password`: `"password tidak boleh sama dengan email"`, `"maksimal 256 byte"`.

400 body bukan JSON atau tipe salah:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": null }, "trace_id": "..." }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

403 developer atau role selain super_admin:

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

403 reset akun sendiri:

```json
{ "error": { "code": "FORBIDDEN", "message": "gunakan ganti password untuk akun sendiri", "details": { "reason": "SELF_ACTION_FORBIDDEN" } }, "trace_id": "..." }
```

403 admin masih ber-flag wajib ganti password:

```json
{ "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null }, "trace_id": "..." }
```

403 tanpa header CSRF: `{ "error": { "code": "CSRF_REQUIRED", ... } }` (format T00).

404 (tidak ada atau `id` bukan uuid):

```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null }, "trace_id": "..." }
```

500 (generik, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

Kode dan pesan 500 mengikuti peta T00; yang dijamin: tidak memuat SQL, hash, atau password.

### Audit (`record.Request`, bukan wire JSON)

Mode dibangkitkan:

```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-...", ActorLabel: "admin@tim.id",
    Action: "auth.password.reset", TargetType: "user", Target: "7d1e5b3a-...",
    Args: map[string]any{"generated": true, "revoked_sessions": 2},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-...",
}
```

Mode diisi admin: sama, `"generated": false`. Tidak ada kunci `password`, `temporary_password`, hash, maupun token di `Args` atau `Result`.
