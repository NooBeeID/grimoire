# T01 core/auth — Usecase `changepassword`

Plan rinci satu usecase. Wajib mengikuti `docs/superpowers/plans/core-auth/overview.md` (selanjutnya "overview"); bila bertentangan, overview menang sampai diperbarui. Sumber aturan: `rules.txt`, `docs/tickets/README.md`, `docs/tickets/T01-core-auth.md`.

Catatan konteks: belum ada `CLAUDE.md`, `.claude/rules/`, maupun knowledge doc module auth. Usecase ini juga PEMILIK penegakan aturan `must_change_password` (middleware `PasswordChangeGate`, daftar exempt, kode `PASSWORD_CHANGE_REQUIRED`, AC4).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (plan ini hanya backend; UI di `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Usecase / folder | `changepassword` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Endpoint | `/api/auth/change-password` |
| Method | POST |
| Version | tanpa prefix versi di URL (`api/modules/core/auth.yml` `info.version: 0.1.0`) |
| Auth | `RequireAuth` (semua role), rate limit per user (`auth.change-password`) |
| Password gate | exempt (boleh diakses saat `must_change_password=true`) |
| Gelombang | B (setelah F1 dan `login`) |
| Endpoint bus | tidak menyediakan. Mengonsumsi `core.audit.record` (Call, via `base/audit`) |
| Job / CLI | tidak ada |
| Bergantung pada | F1 overview (config, `base/*`, `base/store`, `base/audit`, kontrak audit), `login` (supaya ada sesi untuk diuji) |

---

## 2. Summary Plan

User yang sudah login mengganti passwordnya sendiri. Alur:

1. Handler membaca body (`current_password`, `new_password`), meta request, dan hash token sesi saat ini dari cookie.
2. Usecase memuat user (dari `Identity` di ctx), memverifikasi password lama dengan argon2id, memvalidasi password baru lewat rule di `base` (panjang minimum, bukan email, beda dari lama), lalu meng-hash password baru.
3. Repository dalam SATU transaksi: `UPDATE users SET password_hash, must_change_password=false, updated_at` (dengan guard optimistik hash lama), lalu `DELETE FROM sessions WHERE user_id=$1 AND token_hash <> $2` (semua sesi lain dicabut, sesi saat ini tetap hidup).
4. Setelah commit, kirim audit `auth.password.changed` (fail-open) dan balas 204.

Bagian kedua usecase ini adalah penegakan aturan wajib ganti password di infra:

- `PasswordChangeGate` (middleware di grup `/api`, tepat setelah `Session`): user dengan flag `must_change_password=true` hanya boleh mengakses empat endpoint exempt; selain itu 403 `PASSWORD_CHANGE_REQUIRED`.
- Daftar exempt di `lib/constants/auth.go` (`PasswordGateExempt`).
- Kode error `PASSWORD_CHANGE_REQUIRED` sebagai kode bawaan `errs`, dipetakan 403 di peta bawaan `httpx`.

Keputusan desain khusus usecase ini:

| # | Keputusan | Alasan |
|---|---|---|
| C1 | Password lama salah dijawab 400 `INVALID_ARGUMENT` dengan `details.fields.current_password`, BUKAN 401 | 401 memicu logout/redirect otomatis di FE. Sesi tetap sah; yang salah hanya input |
| C2 | Kegagalan verifikasi password lama tidak menaikkan `login_throttles` dan tidak diaudit; pelindungnya rate limit per user (`LOGIN_RATE_PER_MINUTE`) | Pelaku sudah memegang sesi sah. Katalog audit overview tidak punya event untuk ini; tidak menambah event baru |
| C3 | Argon2 (verify dan hash) berjalan DI LUAR transaksi; transaksi hanya berisi UPDATE dan DELETE | Menghindari memegang koneksi dan row lock selama puluhan sampai ratusan ms |
| C4 | Guard optimistik: `UPDATE ... WHERE id=$1 AND password_hash=$oldHash AND is_active`. 0 baris berarti 409 `CONFLICT` | Dua change-password paralel dengan password lama yang sama tidak boleh dua-duanya sukses. Hash lama dipakai sebagai versi |
| C5 | "Beda dari password lama" dibandingkan sebagai plaintext (`subtle.ConstantTimeCompare`) setelah password lama terverifikasi benar, bukan dengan memverifikasi hash kedua | Cukup satu panggilan argon2 verify. Plaintext lama baru valid dibandingkan setelah terbukti benar |
| C6 | Sesi saat ini diidentifikasi dari `HashToken(secret, cookie)` yang dihitung di handler | `Identity` tidak membawa `token_hash`. Cookie pasti ada karena `RequireAuth` lolos; bila tidak ada (tidak seharusnya), jawab 401 defensif |
| C7 | Gate memakai kunci `r.Method + " " + r.URL.Path` persis (bukan route pattern) | Path trailing slash atau varian lain tidak ikut exempt: gagal-aman (diblokir) |

---

## 3. Plan (langkah berurutan)

Aturan umum: TDD per langkah (test dulu). Jangan ubah signature publik T00; hanya menambah. Nama simbol T00 sesuai overview bagian 0 (`httpx` = folder tempat `NewRouter` berada setelah merge). Bila F1 sudah mengerjakan langkah 1 sampai 4 (overview F1 butir 2, 3, 5), jangan duplikasi: verifikasi lalu lanjut ke langkah berikutnya. Plan ini adalah sumber kebenaran isi langkah-langkah itu.

### Langkah 1 — Kode error `PASSWORD_CHANGE_REQUIRED`

1. `backend/lib/common/errs/errs.go`: tambah `CodePasswordChangeRequired Code = "PASSWORD_CHANGE_REQUIRED"`.
2. `backend/internal/infra/<httpx>/status.go`: tambah `errs.CodePasswordChangeRequired: 403` di peta bawaan `codeStatus`. Tidak lewat `RegisterCodeStatus` (D4 overview): kode dipancarkan middleware infra, jadi test infra harus mendapat 403 tanpa wiring domain.
3. Perbarui test peta bawaan (`status_test.go`) dan `errs_test.go` bila memeriksa daftar kode.

### Langkah 2 — Konstanta exempt

`backend/lib/constants/auth.go`:

```go
package constants

// Endpoint yang tetap boleh diakses selama must_change_password=true.
// Key: "METHOD path" persis seperti r.Method + " " + r.URL.Path.
var PasswordGateExempt = map[string]struct{}{
    "POST /api/auth/login":           {},
    "GET /api/auth/me":               {},
    "POST /api/auth/change-password": {},
    "POST /api/auth/logout":          {},
}
```

`backend/lib/constants/auth_test.go`: setiap kunci cocok regex `^(GET|POST|PATCH|PUT|DELETE) /api/\S+$`; tepat empat entri (menambah exempt harus disengaja, test ini ikut diubah di PR yang menambah).

### Langkah 3 — Middleware `PasswordChangeGate`

`backend/internal/infra/<httpx>/passwordgate.go`:

```go
// PasswordChangeGate memblokir user ber-flag must_change_password kecuali endpoint exempt.
func PasswordChangeGate(exempt map[string]struct{}) func(http.Handler) http.Handler
```

Cara kerja (urut):
1. Ambil `Identity` dari ctx lewat `reqctx.IdentityFrom`. Tidak ada identity, atau `MustChangePassword == false`: `next.ServeHTTP`.
2. `key := r.Method + " " + r.URL.Path`. Ada di `exempt`: lanjut.
3. Selain itu: `WriteError(w, r, errs.New(errs.CodePasswordChangeRequired, "password wajib diganti sebelum melanjutkan"))` (403), tanpa memanggil `next`.

Aturan:
- Infra hanya meng-import `lib/constants`, `lib/common/errs`, `lib/reqctx` (rule 4.6). Tidak meng-import domain.
- Map yang diterima disalin saat konstruksi (hindari mutasi eksternal) dan dibaca read-only.
- Path `/api` yang tidak dikenal, diakses user ber-flag, dijawab 403 (bukan 404). Disengaja (overview 3.6).
- Request tanpa identity (anonim, cookie tidak valid) lolos gate; `RequireAuth` per route yang menolaknya 401.

`backend/internal/infra/<httpx>/router.go`: pada grup `/api`, pasang tepat setelah `Session`:
`api.Use(Timeout(...), CSRF(), Session(b, cfg.CookieSecure), PasswordChangeGate(constants.PasswordGateExempt))`. Signature `NewRouter` tidak berubah. Urutan harus tepat: gate sebelum `RequireAuth`/handler route, setelah `Session` (butuh identity).

### Langkah 4 — Rule entity di `base/`

Tambahan di `base/password.go` (milik F1; tambahan aditif, test masuk `password_test.go`). Murni, tanpa I/O:

```go
// DifferentPassword memastikan password baru tidak sama dengan password lama (plaintext, constant time).
// Dipanggil HANYA setelah password lama terverifikasi benar. Sama -> FieldError{"new_password","password baru harus berbeda dari password lama"}.
func DifferentPassword(current, next Secret) error

// ValidateChange menggabungkan aturan password baru. Mengembalikan FieldErrors (nil bila lolos):
//   - policy.ValidateField("new_password", next, email): min length, maks 256 byte, != email (EqualFold)
//   - current kosong -> {"current_password": "wajib diisi"}; next kosong -> {"new_password": "wajib diisi"}
func (p PasswordPolicy) ValidateChange(current, next Secret, email string) FieldErrors
```

Pesan field (konstan, dipakai test dan FE):

| Field | Kondisi | Pesan |
|---|---|---|
| `current_password` | kosong | `wajib diisi` |
| `current_password` | lebih dari 256 byte | `maksimal 256 byte` |
| `current_password` | verifikasi hash gagal | `password lama salah` |
| `new_password` | kosong | `wajib diisi` |
| `new_password` | kurang dari `MinLength` rune | `minimal {n} karakter` |
| `new_password` | lebih dari 256 byte | `maksimal 256 byte` |
| `new_password` | sama dengan email (tanpa beda huruf) | `password tidak boleh sama dengan email` |
| `new_password` | sama dengan password lama | `password baru harus berbeda dari password lama` |

### Langkah 5 — Folder feature `changepassword/`

Path: `backend/internal/domain/core/internal/auth/changepassword/`. Satu usecase, satu folder; jangan dipecah lagi.

**`params.go`**

```go
type request struct {
    CurrentPassword base.Secret `json:"current_password"`
    NewPassword     base.Secret `json:"new_password"`
}
```

`base.Secret` memastikan field tidak bocor lewat `%v`, slog, atau marshal ulang (`"***"`). Tidak ada tipe response (204).

**`repository.go`**

```go
type Repo struct { db *sqlx.DB /* via db.Querier + BeginTx */; store *store.Store }
func NewRepo(d db.DB) *Repo

// FindUserByID membungkus store.FindUserByID.
func (r *Repo) FindUserByID(ctx context.Context, id string) (base.User, bool, error)

type ChangeInput struct {
    UserID        string
    OldHash       string // guard optimistik
    NewHash       string
    KeepTokenHash []byte // sesi saat ini
    Now           time.Time
}
var ErrStale = errors.New("changepassword: password berubah oleh proses lain")

// Change menjalankan dalam SATU transaksi (BeginTx dipanggil di sini, bukan di usecase):
//  1. UPDATE users SET password_hash=$2, must_change_password=false, updated_at=$4
//       WHERE id=$1 AND password_hash=$3 AND is_active   -> RowsAffected==0 => rollback, ErrStale
//  2. DELETE FROM sessions WHERE user_id=$1 AND token_hash <> $2  -> revoked = RowsAffected
// Mengembalikan revoked.
func (r *Repo) Change(ctx context.Context, in ChangeInput) (revoked int64, err error)
```

Catatan SQL: semua query berparameter, kolom eksplisit (tanpa `SELECT *`). `token_hash <> $2` dengan `$2` bertipe `bytea`. Hanya repository yang menyentuh SQL (rule 5.3). Query ini spesifik feature, jadi tidak ditaruh di `base/store`; `FindUserByID` memakai query bersama.

**`usecase.go`**

```go
type Repository interface {
    FindUserByID(ctx context.Context, id string) (base.User, bool, error)
    Change(ctx context.Context, in ChangeInput) (int64, error)
}

type Input struct {
    CurrentPassword, NewPassword base.Secret
    CurrentTokenHash []byte
    Meta base.RequestMeta
}

type Usecase struct {
    repo     Repository
    hasher   base.Hasher
    audit    audit.Recorder
    policy   base.PasswordPolicy
    clock    base.Clock
}
func (u *Usecase) Execute(ctx context.Context, in Input) error
```

Urutan `Execute` (orkestrasi saja; rule ada di entity, rule 5.5):
1. `id, ok := reqctx.IdentityFrom(ctx)`. Tidak ada: `errs.New(errs.CodeUnauthorized, "perlu login")`. `len(in.CurrentTokenHash)==0`: sama.
2. `user, found, err := repo.FindUserByID(ctx, id)`. Error: `fmt.Errorf("changepassword: cari user: %w", err)`. Tidak ditemukan atau `!user.CanLogin()`: 401 `UNAUTHORIZED` (sesi seharusnya sudah gugur lewat `resolvesession`; ini jalur balapan).
3. `fe := policy.ValidateChange(in.CurrentPassword, in.NewPassword, user.Email)`.
4. Jika `current_password` tidak kosong dan tidak melebihi 256 byte: `okOld, err := hasher.Verify(user.PasswordHash, current.Reveal())`.
   - `err != nil` (hash rusak): error terbungkus (500).
   - `!okOld`: `fe["current_password"] = "password lama salah"`.
   - `okOld`: `if e := base.DifferentPassword(current, next); e != nil` gabungkan ke `fe["new_password"]` (hanya bila `new_password` belum punya error).
5. `len(fe) > 0`: kembalikan `base.InvalidFields(fe)` (400). Semua error field dikumpulkan sekaligus.
6. `newHash, err := hasher.Hash(next.Reveal())`. Error: terbungkus (500).
7. `revoked, err := repo.Change(ctx, ChangeInput{...Now: clock()})`.
   - `errors.Is(err, ErrStale)`: `errs.New(errs.CodeConflict, "password berubah oleh proses lain, muat ulang dan coba lagi")`.
   - error lain: `fmt.Errorf("changepassword: simpan: %w", err)`.
8. Setelah commit: `audit.Record(ctx, audit.Event{Action: "auth.password.changed", TargetType: "user", Target: user.ID, Args: {"revoked_sessions": revoked}, Status: "ok", Meta: in.Meta})`. Actor dari ctx (`Actor` nil). Fail-open: `Record` tidak mengembalikan error.
9. Return nil.

Tidak ada nilai password, hash, atau token di Args, log, atau pesan error.

**`handler.go`**

```go
type Handler struct { uc *Usecase; secret []byte }
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request)
```

1. `httpx.DecodeJSON(w, r, &req)` (batas body dan unknown fields ditangani T00; error jadi 400).
2. `c, err := r.Cookie(httpx.SessionCookieName)`; tidak ada: `WriteError(UNAUTHORIZED)`.
3. `tokenHash := base.HashToken(h.secret, c.Value)`.
4. `meta := base.RequestMeta{IP: httpx.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`.
5. `h.uc.Execute(r.Context(), Input{...})`; error: `httpx.WriteError(w, r, err)`.
6. Sukses: `w.WriteHeader(http.StatusNoContent)`. Tanpa Set-Cookie (sesi saat ini tidak dirotasi).

**`init.go`**

```go
func Init(d app.Deps, s base.Shared) {
    repo := NewRepo(d.DB)
    uc := &Usecase{repo: repo, hasher: s.Hasher, audit: audit.New(d.Bus, d.Log), policy: s.Password, clock: s.Clock}
    h := &Handler{uc: uc, secret: s.Session.Secret}
    n := s.LoginRatePerMinute
    lim := httpx.NewLimiter(d.Ctx, httpx.Rule{
        Name: "auth.change-password", Rate: rate.Limit(float64(n)/60), Burst: max(1, n/4), KeyFunc: httpx.KeyByUser,
    })
    d.API.With(httpx.RequireAuth, httpx.RateLimit(lim)).Post("/auth/change-password", h.ChangePassword)
}
```

Verifikasi saat implementasi: apakah `d.API` sudah ter-mount di `/api` (lalu path `"/auth/change-password"`) atau butuh path penuh; ikuti pola yang dipakai `login` di T00. `KeyByUser` bergantung `Identity`, jadi `RequireAuth` harus berjalan sebelum `RateLimit` (urutan `With` di atas).

### Langkah 6 — Wiring

- `auth/base.go`: tambah satu baris `changepassword.Init(d, shared)` di `Register` (satu baris, minim konflik merge).
- `api/modules/core/auth.yml`: pastikan operasi `changePassword` ada (path, `ChangePasswordRequest`, respons 204/400/401/409/429, header wajib `X-Requested-With`, `security: sessionCookie`). Bila F1 sudah menuliskannya, hanya verifikasi. Tambah contoh error `PASSWORD_CHANGE_REQUIRED` (dipakai FE untuk redirect). Jalankan `make api-bundle` dan `make api-check`. Ini dikerjakan SEBELUM kode handler (rule 11.6).
- Dokumen: di `docs/tickets/T01-core-auth.md` pastikan butir "selama flag aktif..." selaras dengan empat endpoint exempt di atas (`login` juga exempt karena tidak butuh sesi).

### Langkah 7 — Test AC4 dan penutup

Tulis test integrasi (bagian 6) dan jalankan `go vet ./...`, `go test ./...`, `make api-check`, archtest `TestT00_AC4_RepoIsClean`.

---

## 4. Migration

Tidak ada. Memakai tabel `users` dan `sessions` dari migrasi F1 `backend/migrations/core/20261021100000000_create_auth_tables.sql`. Kolom yang dipakai: `users.password_hash`, `users.must_change_password`, `users.updated_at`, `sessions.user_id`, `sessions.token_hash`. Indeks `sessions_user_id_idx` sudah mendukung DELETE per user.

---

## 5. Changes (per file/folder)

Infra dan lib:
- `backend/lib/common/errs/errs.go`, `errs_test.go` (ubah): `CodePasswordChangeRequired`.
- `backend/internal/infra/<httpx>/status.go`, `status_test.go` (ubah): peta bawaan 403.
- `backend/internal/infra/<httpx>/passwordgate.go`, `passwordgate_test.go` (baru).
- `backend/internal/infra/<httpx>/router.go`, `router_test.go` (ubah): pasang gate setelah `Session`; satu kasus urutan middleware.
- `backend/lib/constants/auth.go`, `auth_test.go` (baru).

Domain `backend/internal/domain/core/internal/auth/`:
- `base/password.go`, `base/password_test.go` (ubah, aditif): `DifferentPassword`, `PasswordPolicy.ValidateChange`.
- `changepassword/init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go` (baru).
- `changepassword/usecase_test.go`, `repository_integration_test.go`, `handler_test.go` (baru).
- `base.go` (ubah satu baris): `changepassword.Init(d, shared)`.
- `auth_integration_test.go` (ubah): tambah `TestT01_AC4_MustChangePasswordGate`, dan baris `changepassword` di tabel AC8 dan AC9.

API dan dokumen:
- `api/modules/core/auth.yml` (ubah/verifikasi), `api/openapi.yml` (generated).
- `docs/tickets/T01-core-auth.md` (verifikasi selaras).

Kontrak bus: tidak ada kontrak baru. Event audit memakai `record.Request` dari overview 5.5:

```go
record.Request{
    ActorType: "user", ActorID: "<user id>", ActorLabel: "budi@tim.id",
    Action: "auth.password.changed", TargetType: "user", Target: "<user id>",
    Args: map[string]any{"revoked_sessions": 2},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "<trace id>",
}
```

---

## 6. Test Scenario

Penamaan sesuai rule 13.4. Hasher test memakai `Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}`.

### Unit `base/` (tanpa mock)
- `TestDifferentPassword`: sama persis ditolak; beda satu karakter lolos; beda huruf besar/kecil dianggap beda (password case-sensitive).
- `TestPasswordPolicy_ValidateChange` (tabel): kedua kosong menghasilkan dua error; 9 rune ditolak, 10 diterima; multibyte dihitung per rune; 257 byte ditolak; sama dengan email beda huruf ditolak; current lebih dari 256 byte ditolak; semua lolos menghasilkan nil.

### Unit usecase (`changepassword/usecase_test.go`, fake repository, hasher nyata, recorder fake)
- `TestChangePassword_Success`: `Change` dipanggil sekali dengan `OldHash` = hash user, `NewHash` memverifikasi password baru, `KeepTokenHash` = input; audit dipanggil sekali dengan Action `auth.password.changed` dan `Args["revoked_sessions"]` sama dengan nilai repo.
- `TestChangePassword_WrongCurrent`: error 400 dengan `fields.current_password == "password lama salah"`; `Change` TIDAK dipanggil; audit tidak dipanggil.
- `TestChangePassword_NewTooShort`, `TestChangePassword_NewEqualsEmail`, `TestChangePassword_NewEqualsCurrent`: pesan field benar; `Change` tidak dipanggil.
- `TestChangePassword_CollectsAllFieldErrors`: current salah dan new pendek menghasilkan dua field sekaligus; saat current salah, "harus berbeda dari lama" tidak ikut muncul.
- `TestChangePassword_VerifyCalledOnce`: hasher penghitung; satu `Verify` dan satu `Hash` pada jalur sukses; nol `Hash` pada jalur gagal validasi.
- `TestChangePassword_StaleMapsToConflict`: repo mengembalikan `ErrStale` menghasilkan `CONFLICT`; audit tidak dipanggil.
- `TestChangePassword_RepoErrorWrapped`: error infra dibungkus, tidak memuat password/hash; audit tidak dipanggil.
- `TestChangePassword_UserInactiveOrMissing`: 401 `UNAUTHORIZED`; tanpa identity di ctx juga 401.
- `TestChangePassword_AuditFailureDoesNotFail`: recorder yang gagal diam-diam; Execute tetap nil (fail-open).

### Integrasi repository (Postgres nyata, `db.TestDB`)
- `TestChangeRepo_UpdatesHashAndFlag`: hash baru tersimpan, `must_change_password=false`, `updated_at` = now.
- `TestChangeRepo_RevokesOtherSessionsKeepsCurrent`: user punya 3 sesi; `Change` dengan `KeepTokenHash` sesi 1 menghasilkan revoked=2, sesi 1 tersisa; sesi milik user lain tidak tersentuh.
- `TestChangeRepo_StaleHashRollsBack`: `OldHash` salah menghasilkan `ErrStale`, hash tidak berubah, sesi tidak ada yang terhapus (atomik).
- `TestChangeRepo_InactiveUserNotUpdated`: user nonaktif menghasilkan `ErrStale`.
- `TestChangeRepo_ConcurrentSameOldHash`: dua goroutine memakai `OldHash` sama; tepat satu sukses, satu `ErrStale`.

### Handler (`handler_test.go`)
- `TestHandler_Success204NoBodyNoCookie`: 204, body kosong, tanpa Set-Cookie.
- `TestHandler_NoCookie401`, `TestHandler_BadJSON400`, `TestHandler_UnknownFieldRejected`.
- `TestHandler_HashesCurrentCookie`: token cookie yang dikirim masuk ke usecase sebagai `HashToken(secret, token)`.

### Infra
- `TestPasswordGate_BlocksWhenFlagged`: identity flagged dan `GET /api/users` menghasilkan 403 `PASSWORD_CHANGE_REQUIRED`; `next` tidak dipanggil.
- `TestPasswordGate_AllowsExempt`: empat route exempt lolos (identity flagged).
- `TestPasswordGate_AnonymousPasses` dan `TestPasswordGate_UnflaggedPasses`.
- `TestPasswordGate_MethodMatters`: `GET /api/auth/change-password` dan `POST /api/auth/me` diblokir.
- `TestPasswordGate_TrailingSlashBlocked`: `/api/auth/me/` diblokir.
- `TestPasswordGate_UsesFullPathUnderMount`: dipasang pada grup yang di-mount di `/api`; `r.URL.Path` penuh yang dicocokkan.
- `TestRouter_PasswordGateAfterSession`: bus palsu `resolve-session` mengembalikan flagged; lewat `NewRouter` lengkap `/api/users` menghasilkan 403 `PASSWORD_CHANGE_REQUIRED`, `/api/auth/me` (handler stub) lolos.
- `TestStatusFor_PasswordChangeRequired`: 403 tanpa `RegisterCodeStatus`.
- `TestPasswordGateExempt_Shape` (constants).

### Integrasi lintas module (`auth_integration_test.go`)
- `TestT01_AC4_MustChangePasswordGate`: super_admin dibuat lewat `createuser`/seed dengan `must_change_password=true`, login 200, lalu `GET /api/users` 403 `PASSWORD_CHANGE_REQUIRED` (role super_admin supaya bukan 403 role), `GET /api/auth/me` 200 dengan `must_change_password=true`, `POST /api/auth/change-password` 204, `GET /api/users` 200, `GET /api/auth/me` menampilkan `must_change_password=false`.
- `TestT01_ChangePassword_OtherSessionsRevoked`: user login dua kali (sesi A dan B); ganti password lewat A menghasilkan 204; request dengan B menghasilkan 401; request dengan A tetap 200; login dengan password lama menghasilkan 401 `INVALID_CREDENTIALS`; login dengan password baru 200.
- `TestT01_ChangePassword_ValidationMatrix`: password lama salah, baru 9 karakter, baru sama email, baru sama lama menghasilkan 400 dengan pesan field yang benar; hash di DB tidak berubah dan semua sesi masih ada.
- `TestT01_ChangePassword_RateLimited`: lebih dari burst permintaan cepat dari satu user menghasilkan 429 `RATE_LIMITED` dengan `Retry-After`.
- `TestT01_ChangePassword_RequiresCSRFHeader`: tanpa `X-Requested-With` menghasilkan 403 `CSRF_REQUIRED`.
- `TestT01_ChangePassword_Anonymous401`.
- Tambahan baris di tabel AC8 (`TestT01_AC8_PasswordNeverLoggedOrAudited`): password lama dan baru, token cookie, dan hash PHC tidak muncul di buffer log maupun `record.Request` (Args/Result di-marshal), termasuk pada jalur gagal.
- Tambahan baris di tabel AC9 (`TestT01_AC9_EachActionOneAuditEntry`): change-password sukses menghasilkan tepat satu entri `auth.password.changed`; jalur gagal (400, 409) menghasilkan nol entri.
- `TestAudit_UnregisteredDoesNotFailRequest`: tanpa handler audit, change-password tetap 204 dan log berisi `audit record gagal`.

---

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Admin membuat developer tanpa password; developer login dengan password sementara dan otomatis diarahkan ke `/change-password` (dipicu `must_change_password` dari `/api/auth/me` atau 403 `PASSWORD_CHANGE_REQUIRED`). Developer mengisi password lama dan baru, submit, lalu masuk ke beranda. Menu Users tidak tampil. Reload halaman tetap login.
2. Dari `/profile`, user yang flag-nya sudah false mengganti password; setelah sukses muncul notifikasi sukses; form dikosongkan.
3. Developer login di dua browser context (A dan B). Ganti password di A. Di B, aksi berikutnya menghasilkan 401 dan redirect ke `/login`. A tetap login.
4. Setelah ganti password, logout, login dengan password baru berhasil.
5. Admin dengan flag aktif (akun hasil reset) membuka `/admin/users` langsung: UI mengarahkan ke `/change-password`; setelah ganti, `/admin/users` terbuka.

Negatif:
1. Password lama salah: pesan `password lama salah` di bawah field password lama, user tetap login (tidak diredirect ke `/login`).
2. Password baru 9 karakter: pesan `minimal 10 karakter` pada field password baru.
3. Password baru sama dengan email (beda huruf besar/kecil): pesan `password tidak boleh sama dengan email`.
4. Password baru sama dengan password lama: pesan `password baru harus berbeda dari password lama`.
5. User ber-flag memanggil `GET /api/users` lewat fetch manual: 403 `PASSWORD_CHANGE_REQUIRED`. `GET /api/auth/me`, `POST /api/auth/logout` tetap 200/204.
6. User ber-flag memanggil route `/api` yang tidak ada: 403 `PASSWORD_CHANGE_REQUIRED` (bukan 404).
7. `POST /api/auth/change-password` tanpa cookie: 401. Tanpa `X-Requested-With`: 403 `CSRF_REQUIRED`.
8. Banyak percobaan berturut-turut dengan password lama salah: akhirnya 429 `RATE_LIMITED`; UI menampilkan pesan terlalu banyak percobaan.
9. Dua tab mengirim ganti password bersamaan dengan password lama yang sama: satu 204, satu 409 `CONFLICT`.
10. User dinonaktifkan oleh admin saat halaman ganti password terbuka: submit menghasilkan 401 dan redirect ke `/login`.

---

## 8. Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire` dan cookie `grimoire_session`.

### POST /api/auth/change-password

Request:

```json
{
  "current_password": "sementara-AbC123",
  "new_password": "baru-sangat-rahasia"
}
```

204 No Content (tanpa body, tanpa Set-Cookie). Sesi saat ini tetap valid; semua sesi lain user dicabut; `must_change_password` menjadi `false`.

400 (validasi, semua error field dikumpulkan sekaligus):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": {
      "fields": {
        "current_password": "password lama salah",
        "new_password": "minimal 10 karakter"
      }
    }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Pesan lain untuk `new_password`: `"password tidak boleh sama dengan email"`, `"password baru harus berbeda dari password lama"`, `"maksimal 256 byte"`, `"wajib diisi"`. Untuk `current_password`: `"wajib diisi"`, `"maksimal 256 byte"`.

400 (body bukan JSON valid atau ada field tak dikenal):

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

401 (tanpa sesi, sesi kedaluwarsa, atau user nonaktif):

```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

403 (tanpa header CSRF, dari middleware T00):

```json
{
  "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib ada", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

409 (password berubah oleh permintaan lain):

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "password berubah oleh proses lain, muat ulang dan coba lagi",
    "details": null
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

429 (rate limit per user, dari middleware T00; header `Retry-After: 3`):

```json
{
  "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

500 (error infra; pesan generik, tanpa detail internal):

```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

(Nama kode 500 mengikuti `errs` T00; samakan dengan konstanta yang ada saat implementasi.)

### 403 password gate (endpoint non-exempt, user ber-flag)

Contoh `GET /api/users` oleh user dengan `must_change_password=true`:

```json
{
  "error": {
    "code": "PASSWORD_CHANGE_REQUIRED",
    "message": "password wajib diganti sebelum melanjutkan",
    "details": null
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### GET /api/auth/me (dipakai FE sebelum dan sesudah ganti password; dimiliki `getme`)

Sebelum ganti:

```json
{ "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": true }
```

Sesudah ganti (request berikutnya membaca ulang dari DB lewat `resolve-session`, tanpa cache):

```json
{ "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": false }
```

### Bus

Tidak ada endpoint bus yang disediakan. Satu panggilan keluar: `bus.CallTyped[record.Response](ctx, bus, constants.BUS_CORE_AUDIT_RECORD, record.Request{...})` (contoh isi di bagian 5), dilakukan oleh `base/audit` setelah transaksi commit; kegagalan hanya di-log (`audit record gagal`), tidak menggagalkan request.

---

## 9. Risiko dan catatan

- Kontrak `core.audit.record` dimiliki T02. Bila bentuknya berbeda saat merge, hanya `base/audit` yang disesuaikan; usecase ini tidak berubah.
- Biaya argon2 per request (verify dan hash, sekitar 100 ms dengan m=64 MiB, memori sama per panggilan). Rate limit per user menjadi pelindung; tidak ada semaphore tambahan (konsisten overview 14).
- Seluruh hash lama dengan parameter berbeda tetap terverifikasi (parameter dibaca dari string PHC). Hash baru memakai parameter config saat ini, jadi change-password sekaligus me-rehash.
- Penambahan endpoint exempt baru di masa depan harus mengubah `PasswordGateExempt` dan `auth_test.go` di PR yang sama; sengaja dibuat eksplisit.
- Bentuk path `d.API` dan nama folder `httpx` mengikuti T00 setelah merge; plan merujuk nama simbol, bukan path folder.
