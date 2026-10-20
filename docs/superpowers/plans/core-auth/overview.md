# T01 core/auth — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T01-core-auth.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/foundation/overview.md` (API T00).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. T00 masih di `worktree/feat-foundation` (branch `feat/foundation`, WIP). Tidak ada knowledge doc module auth.

Dokumen ini adalah plan FOUNDATION untuk module `core/auth`: keputusan bersama, `base/`, kontrak bus, migrasi, konfigurasi, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/core-auth/<slug>.md` dan WAJIB mengikuti keputusan di sini. Bila plan usecase bertentangan dengan dokumen ini, dokumen ini yang menang sampai diperbarui.

---

## 0. Prasyarat dan catatan tentang T00

- T01 dikerjakan di atas T00 yang sudah di-merge. Signature publik T00 TIDAK diubah. T01 hanya MENAMBAH: field config, konstanta, kontrak, satu middleware, satu kode error bawaan, satu helper di `cmd/internal/server`.
- Nama package T00 yang dipakai di plan ini (sesuai kode WIP saat ini):

| Simbol | Package | Path import di WIP T00 |
|---|---|---|
| `NewRouter`, `WriteJSON`, `WriteError`, `DecodeJSON`, `RegisterCodeStatus`, `StatusFor`, `Session`, `RequireAuth`, `RequireRole`, `RateLimit`, `NewLimiter`, `Rule`, `KeyByIP`, `KeyByUser`, `SessionCookieName`, `Timeout`, `BodyLimit` | `httpx` | `grimoire/internal/infra/http-server` |
| `Scheduler`, `Job` | `jobs` | `grimoire/internal/infra/jobs-scheduler` |
| `DB`, `BeginTx`, `Querier`, `TestDB`, `Open`, `Up` | `db` | `grimoire/internal/infra/db` |
| `Registry`, `CallTyped`, `Expect`, `ErrNotRegistered` | `bus` | `grimoire/internal/infra/bus` |
| `Deps` | `app` | `grimoire/internal/infra/app` |

  Overview T00 menyebut folder `infra/http` dan `infra/middleware`. Di WIP, semua middleware ada di package `httpx`. Bila folder di-rename saat T00 merge, yang berubah hanya path import; nama simbol tetap. Di plan ini, "package `httpx`" berarti folder tempat `NewRouter` berada setelah merge.
- `httpx.SessionCookieName = "grimoire_session"` sudah ada. T01 memakainya, tidak mendefinisikan ulang.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (backend Go di plan ini; frontend di `frontend-plan.md` terpisah) |
| Domain | core |
| Module | auth (`backend/internal/domain/core/internal/auth`) |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Versi kontrak REST | Tanpa prefix versi di URL. `api/modules/core/auth.yml` memakai `info.version: 0.1.0` |
| Bergantung pada | T00 (merged). Paralel dengan T02 (kontrak `core.audit.record`, lihat 5.5) |

Endpoint HTTP:

| Endpoint | Method | Version | Auth | Password gate | Usecase |
|---|---|---|---|---|---|
| `/api/auth/login` | POST | - | publik, rate limit per IP | exempt | `login` |
| `/api/auth/logout` | POST | - | `RequireAuth` | exempt | `logout` |
| `/api/auth/me` | GET | - | `RequireAuth` | exempt | `getme` |
| `/api/auth/change-password` | POST | - | `RequireAuth`, rate limit per user | exempt | `changepassword` |
| `/api/users` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `createuser` |
| `/api/users` | GET | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `listusers` |
| `/api/users/{id}` | GET | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `getuser` |
| `/api/users/{id}` | PATCH | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `updateuser` |
| `/api/users/{id}/deactivate` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `setuserstatus` |
| `/api/users/{id}/activate` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `setuserstatus` |
| `/api/users/{id}/reset-password` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | dicek | `resetpassword` |

Endpoint bus (provider T01):

| Endpoint | Mode | Version | Consumer | Usecase |
|---|---|---|---|---|
| `core.auth.resolve-session` | Call | kontrak T00 | infra session middleware (T00) | `resolvesession` |
| `core.auth.get-user` | Call | baru | T03, T04, T05, T06 | `getuserbus` |
| `core.auth.find-developers` | Call | baru | T03 | `finddevelopers` |
| `core.auth.create-admin` | Call | baru | `cmd/cli` | `createadmin` |

Job: `core.auth.purge-sessions` (DailyAt `03:00` UTC) — usecase `purgesessions`.
CLI: `./grimoire create-admin --email E --name N` — usecase `createadmin`.

---

## 2. Summary Plan

T01 membangun akun user dua role (`super_admin`, `developer`), login email+password dengan cookie sesi, dan manajemen user oleh Super Admin. Isinya:

- Tiga tabel milik module: `users`, `sessions`, `login_throttles`, dalam satu migrasi.
- Entity dan rule murni di `base/`: normalisasi dan validasi user, rule penjaga admin, kebijakan password plus hasher argon2id, token sesi plus HMAC, dan throttle login.
- Dua subpackage I/O bersama di bawah `base/`: `base/store` (query SQL yang identik lintas feature) dan `base/audit` (pengirim audit lewat bus).
- 15 feature: 11 titik masuk HTTP/CLI, 3 handler bus, 1 job.
- Satu middleware infra baru (`PasswordChangeGate`) untuk menegakkan `must_change_password`.
- Tiga kontrak bus baru, dan kontrak `core.audit.record` yang disepakati dengan T02.
- 10 env var baru dan spesifikasi OpenAPI `api/modules/core/auth.yml`.

Hal yang penting soal keamanan:
- Hanya hash yang disimpan: argon2id untuk password, dan HMAC-SHA256 dengan `SESSION_SECRET` untuk token sesi.
- Status user dicek di setiap request lewat `resolve-session`.
- Lockout berlaku per email, baik email yang terdaftar maupun tidak.
- Respons gagal login seragam dan waktunya disamakan dengan dummy hash.
- Password tidak pernah muncul di log, audit, atau error.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Penyimpangan dari tiket/desain dekomposisi (dicatat di PR)

| # | Keputusan | Alasan | Tindakan dokumen |
|---|---|---|---|
| D1 | `failed_login_count`/`locked_until` TIDAK di `users`, tetapi di tabel `login_throttles` yang dikunci per email (email terdaftar maupun tidak) | Respons gagal tidak boleh membedakan email tidak ada vs password salah. Bila lockout hanya untuk email terdaftar, respons "terkunci" membocorkan keberadaan akun | Ubah bagian "Model data" `T01-core-auth.md` di PR yang sama |
| D2 | Audit T01 dikirim dengan `bus.Call` (bukan `Publish` seperti tiket) setelah transaksi commit, fail-open | rules.txt 6.3 mewajibkan Call untuk aksi keamanan; T02 memutuskan fail-open | Ubah kalimat "Dikonsumsi" di `T01-core-auth.md` |
| D3 | `base/` dipecah: `base/` (package `base`, murni, tanpa I/O), `base/store/` (SQL bersama), `base/audit/` (kirim audit via bus) | rules.txt 5.1 melarang I/O dan SQL di entity. Archtest T00 menganggap semua path di bawah `base/` sebagai milik module (boleh di-import feature) | Tidak perlu ubah rules |
| D4 | `PASSWORD_CHANGE_REQUIRED` menjadi kode bawaan `errs` dan dipetakan ke 403 di peta bawaan `httpx` (bukan `RegisterCodeStatus` dari auth) | Kode itu dipancarkan oleh middleware infra. Test infra harus mendapat 403 tanpa wiring domain | - |
| D5 | Kode `INVALID_CREDENTIALS` (401) dan `ACCOUNT_LOCKED` (429) milik auth, didaftarkan via `httpx.RegisterCodeStatus` dibungkus `sync.Once` | `RegisterCodeStatus` panic bila duplikat, sedangkan test integrasi dan CLI bisa memanggil `auth.Register` lebih dari sekali per proses | - |
| D6 | Tanpa indeks `lower(name)` | ILIKE `%q%` tidak memakai btree. Jumlah user puluhan (desain), jadi seq scan wajar. Rule 8.8 hanya untuk query berat | - |
| D7 | Tambah katalog bus `core.auth.create-admin` | Consumer `cmd/cli` (TODO dari T00) | Ubah `docs/tickets/README.md` |

### 3.2 Library baru

| Kebutuhan | Pilihan | Alasan |
|---|---|---|
| argon2id | `golang.org/x/crypto/argon2` | Implementasi resmi tim Go, dipelihara aktif (rule 15.4) |
| HMAC, rand, constant-time | stdlib `crypto/hmac`, `crypto/sha256`, `crypto/rand`, `crypto/subtle` | - |
| UUID parse | `github.com/google/uuid` (sudah ada di T00) | - |
| Unique violation | `github.com/jackc/pgx/v5/pgconn` (`*pgconn.PgError`, `Code == "23505"`) via `errors.As` | pgx sudah dipakai T00 |

### 3.3 Struktur module

```
backend/internal/domain/core/internal/auth/
  base.go                       # package auth: Register(d app.Deps)
  base/                         # package base: entity + rule, TANPA I/O, TANPA context
    user.go  user_test.go
    adminguard.go  adminguard_test.go
    password.go  password_test.go       # ValidatePassword, Secret, Hasher, Argon2Hasher, GeneratePassword
    session.go  session_test.go         # NewSessionToken, HashToken, Session
    throttle.go  throttle_test.go       # LoginThrottle, ThrottlePolicy
    like.go  like_test.go               # LikePattern (escape ILIKE)
    errors.go                           # kode dan konstruktor error module
    clock.go                            # type Clock func() time.Time
    meta.go                             # RequestMeta{IP, UserAgent}
    deps.go                             # Shared (dependensi bersama yang diteruskan ke feature Init)
  base/store/                   # package store: SQL bersama (rule 5.3 "repository/")
    store.go  store_integration_test.go
  base/audit/                   # package audit: builder + pengirim audit via bus
    audit.go  audit_test.go
  login/ logout/ getme/ changepassword/ createuser/ listusers/ getuser/ updateuser/
  setuserstatus/ resetpassword/ createadmin/ resolvesession/ getuserbus/ finddevelopers/ purgesessions/
    init.go handler.go params.go usecase.go repository.go *_test.go
```

Aturan per feature:
- `init.go`: `func Init(d app.Deps, s base.Shared)`. Fungsi ini merakit repository, usecase, dan handler, lalu mendaftarkan route, bus, atau job feature itu sendiri.
- Feature bus (`resolvesession`, `getuserbus`, `finddevelopers`, `createadmin`) tidak punya `params.go` HTTP. `handler.go` berisi handler bus yang memanggil `bus.Expect[contract.Request](req)` (rule 6.9).
- `purgesessions` tidak punya `handler.go` atau `params.go`. `init.go` mendaftarkan `jobs.Job`.
- `getme` tidak punya `repository.go` (dibaca dari `reqctx.Identity`).
- Usecase menerima interface (repository, `base.Hasher`, `audit.Recorder`, `base.Clock`). Bukan tipe konkret (rule 5.6).
- Feature DILARANG saling import (rule 4.3); yang dipakai bersama lewat `base`, `base/store`, `base/audit`.

### 3.4 `base.Shared` (dibangun sekali di `auth/base.go`)

```go
// base/deps.go
type SessionConfig struct {
    Secret          []byte        // []byte(cfg.SessionSecret)
    IdleTimeout     time.Duration // SESSION_IDLE_TIMEOUT
    AbsoluteTimeout time.Duration // SESSION_ABSOLUTE_TIMEOUT
    CookieSecure    bool          // COOKIE_SECURE
}

type Shared struct {
    Hasher      Hasher          // *Argon2Hasher dari config
    DummyHash   string          // Hasher.Hash("grimoire-dummy-password") dihitung sekali saat Register
    Clock       Clock           // time.Now().UTC di produksi
    Rand        io.Reader       // crypto/rand.Reader di produksi
    Session     SessionConfig
    Password    PasswordPolicy  // {MinLength: PASSWORD_MIN_LENGTH}
    Throttle    ThrottlePolicy  // {MaxFailures, Window, LockDuration}
    LoginRatePerMinute int      // LOGIN_RATE_PER_MINUTE
}
```

`Shared` hanya berisi nilai dan dependensi tanpa I/O. Store dan auditor TIDAK ditaruh di `Shared` karena package `base` tidak boleh meng-import `base/store` (I/O). Masing-masing feature `Init` membangun sendiri `store.New(d.DB)` dan `audit.New(d.Bus, d.Log)`. Biayanya murah (struct kecil) dan tidak ada state.

### 3.5 Konvensi request meta, actor, dan log

- Handler membangun `base.RequestMeta{IP: httpx.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`, lalu meneruskannya ke usecase. Usecase tidak menyentuh `*http.Request`.
- Actor diambil dari `reqctx.IdentityFrom(ctx)` di usecase. Untuk audit, `actor_label` diisi email actor.
- Tipe `base.Secret` (string) dipakai untuk semua field password di `params.go`:
  - `String()` dan `LogValue()` mengembalikan `"***"`.
  - `MarshalJSON` mengembalikan `"***"`.
  - Unmarshal tetap memakai perilaku bawaan string.
  - `Reveal() string` dipakai hanya saat memanggil hasher.
- Respons yang membawa `temporary_password` memasang header `Cache-Control: no-store`.
- Error ke klien tidak pernah memuat password, token, hash, atau SQL. Usecase membungkus error infra dengan `fmt.Errorf("<feature>: <langkah>: %w", err)`. Pesan klien untuk error infra tetap generik (500).

### 3.6 Penegakan `must_change_password` (pemilik aturan: `changepassword`)

- File baru `httpx/passwordgate.go`: `func PasswordChangeGate(exempt map[string]struct{}) func(http.Handler) http.Handler`.
- Cara kerja:
  1. Tidak ada `Identity`, atau `Identity.MustChangePassword == false`: request lanjut.
  2. Kunci `r.Method + " " + r.URL.Path` ada di `exempt`: request lanjut.
  3. Selain itu: `WriteError(errs.New(errs.CodePasswordChangeRequired, "password wajib diganti sebelum melanjutkan"))`, menghasilkan 403.
- Dipasang di `httpx.NewRouter` pada grup `/api` tepat setelah `Session`, yaitu `api.Use(Timeout(...), CSRF(), Session(b, cfg.CookieSecure), PasswordChangeGate(constants.PasswordGateExempt))`. Signature `NewRouter` tidak berubah.
- `lib/constants/auth.go` berisi:

```go
// Endpoint yang tetap boleh diakses selama must_change_password=true. Key: "METHOD path" persis.
var PasswordGateExempt = map[string]struct{}{
    "POST /api/auth/login":           {},
    "GET /api/auth/me":               {},
    "POST /api/auth/change-password": {},
    "POST /api/auth/logout":          {},
}
```

- Infra hanya meng-import `lib/constants` dan `lib/common/errs`, tidak pernah domain (rule 4.6).
- Route `/api` yang tidak dikenal, diakses user ber-flag, dijawab 403 `PASSWORD_CHANGE_REQUIRED` (bukan 404). Ini disengaja dan cukup.

### 3.7 Kebijakan login (dipakai `login`, diuji AC2/AC3)

Urutan di usecase `login`:
1. `email := strings.ToLower(strings.TrimSpace(in.Email))`. Kosong atau lebih dari 254 byte: 400 `INVALID_ARGUMENT`. Password kosong atau lebih dari 256 byte: 400. Format email TIDAK divalidasi di login. Menolak format tidak membocorkan keberadaan akun, tetapi juga tidak perlu.
2. `throttle := repo.GetThrottle(email)` (tanpa lock). Bila `throttle.IsLocked(now)`:
   - respons 429 `ACCOUNT_LOCKED` dengan `details.retry_after_seconds = ceil(LockedUntil-now)` dan header `Retry-After`;
   - audit `auth.login.failed` dengan reason `locked`;
   - TIDAK memanggil hasher dan TIDAK menaikkan counter (lock tidak diperpanjang oleh percobaan saat terkunci).
3. `user, found := store.FindUserByEmail(email)`. Hash yang diverifikasi adalah `user.PasswordHash` bila found, `Shared.DummyHash` bila tidak. `ok := hasher.Verify(hash, pw)` SELALU dipanggil tepat satu kali.
4. `success = found && ok && user.CanLogin()`. Bila gagal:
   - transaksi `RegisterFailure`: `INSERT ... ON CONFLICT DO NOTHING`, lalu `SELECT ... FOR UPDATE`, entity `RegisterFailure(now, policy)`, lalu `UPDATE`;
   - respons 401 `INVALID_CREDENTIALS`, pesan `"email atau password salah"`;
   - audit `auth.login.failed`. Reason `invalid_credentials` (email tidak ada atau password salah) atau `inactive` (password benar tetapi user nonaktif). Reason hanya ada di audit, tidak di respons.
5. Bila sukses, satu transaksi berisi:
   - `DELETE FROM login_throttles WHERE email=$1`;
   - `INSERT sessions` (token baru 32 byte, simpan `HashToken`);
   - `UPDATE users SET last_login_at=now()`.

   Setelah commit: set cookie, respons 200, audit `auth.login.success`.
6. Cookie: `Name=httpx.SessionCookieName`, `Value=<token>`, `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure=COOKIE_SECURE`, `MaxAge=int(SESSION_ABSOLUTE_TIMEOUT.Seconds())`.

Rate limit per IP: `httpx.NewLimiter(d.Ctx, httpx.Rule{Name: "auth.login", Rate: rate.Limit(float64(n)/60), Burst: max(1, n/4), KeyFunc: httpx.KeyByIP})` dengan `n = LOGIN_RATE_PER_MINUTE`. Change-password memakai aturan yang sama dengan `Name: "auth.change-password"` dan `KeyFunc: httpx.KeyByUser`.

### 3.8 Kebijakan sesi

- Token: 32 byte dari `Shared.Rand`, di-encode `base64.RawURLEncoding` (43 karakter). Yang disimpan hanya `HashToken(secret, token) = HMAC-SHA256(SESSION_SECRET, token)` (32 byte, `bytea`).
- Valid bila `now < ExpiresAt && now < LastSeenAt + IdleTimeout`. `ExpiresAt = CreatedAt + AbsoluteTimeout`.
- Sesi juga harus milik user aktif; pengecekan ini di usecase `resolvesession` lewat `user.CanLogin()`.
- Sentuhan: `NeedsTouch(now) = now.Sub(LastSeenAt) >= time.Minute`. Repository menjalankan `UPDATE sessions SET last_seen_at=$2 WHERE id=$1 AND last_seen_at < $2 - interval '1 minute'`, idempoten saat request paralel.
- Sesi kedaluwarsa tidak dihapus di jalur request; penghapusan dilakukan `purgesessions`.
- Pencabutan sesi:

| Pemicu | Sesi yang dihapus | Transaksi |
|---|---|---|
| `logout` | sesi saat ini | sendiri |
| `changepassword` | semua sesi user kecuali sesi saat ini (`token_hash <> $2`) | sama dengan update password |
| `resetpassword` | semua sesi user target | sama dengan update password |
| `setuserstatus` (deactivate) | semua sesi user target | sama dengan update status |
| `updateuser` (role berubah) | tidak ada | role dibaca ulang di setiap request |

### 3.9 Penjaga Super Admin (dipakai `updateuser`, `setuserstatus`)

Di dalam satu transaksi:
1. `SELECT ... FROM users WHERE id=$1 FOR UPDATE` (target).
2. `SELECT id FROM users WHERE role='super_admin' AND is_active FOR UPDATE`. Jumlah baris menjadi `activeSuperAdminCount`. `FOR UPDATE` tidak boleh dipakai bersama agregat, jadi penghitungan dilakukan di Go.
3. Panggil rule entity `CheckRoleChange` atau `CheckDeactivate`.
4. `UPDATE`.

Dengan READ COMMITTED, dua admin yang saling menonaktifkan akan saling menunggu lock. Transaksi kedua membaca ulang baris dan menjumpai hanya satu super admin aktif, sehingga mendapat `LAST_SUPER_ADMIN`.

Pemetaan error:
- Aksi pada diri sendiri: 403 `FORBIDDEN`, `details.reason = "SELF_ACTION_FORBIDDEN"`, pesan `"tidak boleh menonaktifkan atau menurunkan role akun sendiri"`.
- Super admin aktif terakhir: 409 `CONFLICT`, `details.reason = "LAST_SUPER_ADMIN"`, pesan `"minimal harus ada satu super admin aktif"`.

### 3.10 Password awal/sementara

- `createuser` dan `resetpassword`: field `password` opsional.
  - Bila kosong, server memanggil `GeneratePassword(Shared.Rand)` dan mengembalikan hasilnya SEKALI di `temporary_password`.
  - Bila diisi, password divalidasi `ValidatePassword` dan TIDAK dikembalikan.
  - Keduanya mengeset `must_change_password=true`.
- `createadmin`: password wajib (dari CLI) dan divalidasi. Mengeset `must_change_password=false` karena operator sendiri yang memilihnya.
- `changepassword`: verifikasi password lama, pastikan password baru berbeda dari yang lama, jalankan `ValidatePassword`, lalu set `must_change_password=false`.
- Hash lama dengan parameter argon2 berbeda tetap terverifikasi karena parameter dibaca dari string PHC. Rehash otomatis saat login TIDAK dilakukan (di luar scope).

### 3.11 Email unik

- `email citext UNIQUE`. Aplikasi tetap menyimpan hasil `NormalizeEmail` (lowercase).
- Unique violation `23505` pada constraint `users_email_key` dipetakan di repository ke `base.ErrEmailTaken`. Usecase mengubahnya menjadi `errs.New(CONFLICT, "email sudah terdaftar")`. Berlaku di `createuser`, `updateuser`, `createadmin`.

### 3.12 Paginasi dan pencarian

- `listusers` memakai limit/offset (rule 11.5): default 20, maksimum 100 (di atas itu 400), offset >= 0. Urutan `name ASC, id ASC`. `total` dari `COUNT(*) OVER()`.
- Pencarian dengan `base.LikePattern(q)`:
  - escape `\` menjadi `\\`, `%` menjadi `\%`, `_` menjadi `\_`;
  - dibungkus `%...%`;
  - SQL: `(email ILIKE $n ESCAPE '\' OR name ILIKE $n ESCAPE '\')`.

  Dipakai `listusers` dan `finddevelopers`.
- Path `{id}` yang bukan uuid valid: 404 `NOT_FOUND` `"user tidak ditemukan"` (sama dengan tidak ada).

---

## 4. Entity dan rule (`base/`, package `base`)

Semua fungsi murni: tanpa context, tanpa I/O kecuali `io.Reader` yang diinjeksi untuk keacakan. Diuji unit tanpa mock (rule 13.1).

### 4.1 `base/user.go`

```go
import "grimoire/lib/contracts/core/auth/resolvesession" // RoleSuperAdmin, RoleDeveloper

type User struct {
    ID                 string
    Email              string
    Name               string
    PasswordHash       string
    Role               string
    IsActive           bool
    MustChangePassword bool
    CreatedAt          time.Time
    CreatedBy          *string
    CreatedByLabel     *string
    UpdatedAt          time.Time
    LastLoginAt        *time.Time
}

func NormalizeEmail(s string) (string, error)  // trim + lower; 3..254 byte; net/mail.ParseAddress dan hasil .Address == input (tanpa nama tampilan); gagal -> FieldError{"email", "format email tidak valid"}
func ValidateName(s string) (string, error)    // trim; 1..100 rune; tanpa karakter kontrol; gagal -> FieldError{"name", ...}
func ValidRole(r string) bool                  // super_admin | developer
func (u User) CanLogin() bool                  // u.IsActive
func (u User) ToResponse() UserDTO             // tanpa PasswordHash

type UserPatch struct{ Name, Email, Role *string }
// ApplyUpdate memvalidasi dan menerapkan patch; field yang nilainya sama tidak dihitung berubah.
// Mengembalikan daftar field berubah berurutan ["name","email","role"] dan FieldErrors gabungan.
func (u *User) ApplyUpdate(p UserPatch) (changed []string, err error)

type UserDTO struct {
    ID                 string     `json:"id"`
    Email              string     `json:"email"`
    Name               string     `json:"name"`
    Role               string     `json:"role"`
    IsActive           bool       `json:"is_active"`
    MustChangePassword bool       `json:"must_change_password"`
    CreatedAt          time.Time  `json:"created_at"`
    CreatedBy          *string    `json:"created_by"`
    CreatedByLabel     *string    `json:"created_by_label"`
    UpdatedAt          time.Time  `json:"updated_at"`
    LastLoginAt        *time.Time `json:"last_login_at"`
}

type MeDTO struct { ID, Email, Name, Role string; MustChangePassword bool } // tag json snake_case
```

`FieldErrors` (`map[string]string`) mengimplementasikan `error`. Usecase mengubahnya menjadi `errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid", map[string]any{"fields": fe})`. Helper konversinya `base.InvalidFields(fe)` ada di `errors.go`.

### 4.2 `base/adminguard.go`

```go
const (
    ReasonSelfAction      = "SELF_ACTION_FORBIDDEN"
    ReasonLastSuperAdmin  = "LAST_SUPER_ADMIN"
)
// Dicek berurutan: self lebih dulu, lalu last-admin.
func CheckRoleChange(actorID string, target User, newRole string, activeSuperAdminCount int) error
//   newRole == target.Role                                         -> nil
//   target.ID == actorID                                           -> ErrSelfAction
//   target.Role==super_admin && target.IsActive && count <= 1      -> ErrLastSuperAdmin
func CheckDeactivate(actorID string, target User, activeSuperAdminCount int) error
//   !target.IsActive                                               -> nil (idempoten; usecase tidak menulis)
//   target.ID == actorID                                           -> ErrSelfAction
//   target.Role==super_admin && count <= 1                         -> ErrLastSuperAdmin
```

`ErrSelfAction` dan `ErrLastSuperAdmin` adalah `*errs.Error` siap pakai dari `errors.go` (3.9).

### 4.3 `base/password.go`

```go
type PasswordPolicy struct{ MinLength int }
const MaxPasswordBytes = 256
func (p PasswordPolicy) Validate(pw, email string) error
//   utf8.RuneCountInString(pw) < MinLength -> FieldError "minimal %d karakter"
//   len(pw) > 256                          -> "maksimal 256 byte"
//   strings.EqualFold(pw, email)           -> "password tidak boleh sama dengan email"
//   field name diberikan pemanggil lewat ValidateField(field, pw, email)

type Secret string // String/LogValue/MarshalJSON -> "***"; Reveal() string

type Hasher interface {
    Hash(pw string) (string, error)
    Verify(encoded, pw string) (bool, error) // error hanya untuk hash rusak; mismatch -> (false, nil)
}
type Argon2Params struct{ MemoryKiB uint32; Iterations uint32; Parallelism uint8 }
type Argon2Hasher struct{ Params Argon2Params; Rand io.Reader }
// Hash: salt 16 byte dari Rand, argon2.IDKey(pw, salt, t, m, p, 32)
//   format: $argon2id$v=19$m=65536,t=3,p=2$<b64 salt RawStd>$<b64 key RawStd>
// Verify: parse PHC (param dari string, bukan config), hitung ulang, subtle.ConstantTimeCompare

const generatedAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789" // tanpa 0 O 1 l I
func GeneratePassword(r io.Reader) (string, error) // 16 karakter, rejection sampling agar tidak bias
```

### 4.4 `base/session.go`

```go
const TokenBytes = 32
const TouchInterval = time.Minute
func NewSessionToken(r io.Reader) (string, error)   // base64.RawURLEncoding(32 byte)
func HashToken(secret []byte, token string) []byte  // HMAC-SHA256
type Session struct {
    ID, UserID string
    TokenHash  []byte
    CreatedAt, LastSeenAt, ExpiresAt time.Time
    IP, UserAgent *string
}
func NewSession(userID string, tokenHash []byte, now time.Time, cfg SessionConfig, meta RequestMeta) Session
func (s Session) IsValid(now time.Time, idle time.Duration) bool
func (s Session) NeedsTouch(now time.Time) bool
func (s Session) MaxAgeSeconds(cfg SessionConfig) int // untuk cookie
```

### 4.5 `base/throttle.go`

```go
type ThrottlePolicy struct{ MaxFailures int; Window, LockDuration time.Duration }
type LoginThrottle struct {
    Email           string
    FailedCount     int
    WindowStartedAt time.Time
    LockedUntil     *time.Time
}
func (t LoginThrottle) IsLocked(now time.Time) bool                  // LockedUntil != nil && now < *LockedUntil
func (t LoginThrottle) RetryAfter(now time.Time) time.Duration       // 0 bila tidak terkunci
func (t *LoginThrottle) RegisterFailure(now time.Time, p ThrottlePolicy)
//   bila WindowStartedAt nol atau now >= WindowStartedAt+Window -> reset: FailedCount=0, WindowStartedAt=now, LockedUntil=nil
//   FailedCount++
//   bila FailedCount >= MaxFailures -> LockedUntil = now+LockDuration
func (t LoginThrottle) Expired(now time.Time, p ThrottlePolicy) bool // untuk test kebijakan purge (cermin SQL purge)
```

### 4.6 `base/errors.go`

```go
const (
    CodeInvalidCredentials errs.Code = "INVALID_CREDENTIALS" // 401
    CodeAccountLocked      errs.Code = "ACCOUNT_LOCKED"      // 429
)
var (
    ErrEmailTaken     = errors.New("auth: email sudah terdaftar")  // sentinel repository
    ErrUserNotFound   = errs.New(errs.CodeNotFound, "user tidak ditemukan")
    ErrInvalidCreds   = errs.New(CodeInvalidCredentials, "email atau password salah")
    ErrSelfAction     = errs.WithDetails(errs.CodeForbidden, "tidak boleh menonaktifkan atau menurunkan role akun sendiri", map[string]any{"reason": ReasonSelfAction})
    ErrLastSuperAdmin = errs.WithDetails(errs.CodeConflict, "minimal harus ada satu super admin aktif", map[string]any{"reason": ReasonLastSuperAdmin})
)
func EmailConflict() *errs.Error                                       // CONFLICT "email sudah terdaftar"
func AccountLocked(retry time.Duration) *errs.Error                    // details {retry_after_seconds}
func InvalidFields(fe FieldErrors) *errs.Error
```

Pendaftaran status HTTP untuk dua kode di atas TIDAK ada di `base` (package `base` tidak boleh meng-import `httpx`). Pendaftarannya ada di `auth/base.go` sebagai `registerCodes()`, dibungkus `sync.Once`, berisi `httpx.RegisterCodeStatus(base.CodeInvalidCredentials, 401)` dan `httpx.RegisterCodeStatus(base.CodeAccountLocked, 429)`.

### 4.7 `base/like.go`, `base/clock.go`, `base/meta.go`

- `func LikePattern(q string) string` (3.12).
- `type Clock func() time.Time`.
- `type RequestMeta struct{ IP, UserAgent string }`.

---

## 5. Kontrak bus dan konstanta endpoint

### 5.1 `backend/lib/constants/bus.go` (ditambah)

```go
// Provider: T01 core/auth.
const BUS_CORE_AUTH_RESOLVE_SESSION = "core.auth.resolve-session" // sudah ada (T00)
const BUS_CORE_AUTH_GET_USER        = "core.auth.get-user"
const BUS_CORE_AUTH_FIND_DEVELOPERS = "core.auth.find-developers"
const BUS_CORE_AUTH_CREATE_ADMIN    = "core.auth.create-admin"

// Provider: T02 core/auditlog. Dibuat oleh tiket yang merge lebih dulu (5.5).
const BUS_CORE_AUDIT_RECORD = "core.audit.record"
```

`lib/constants/bus_test.go` milik T00 (bila memeriksa pola nama atau keunikan nilai) harus tetap lulus.

### 5.2 `lib/contracts/core/auth/getuser/getuser.go`

```go
package getuser
type Request struct{ UserID string }
type Response struct {
    Found    bool
    ID       string
    Name     string
    Email    string
    Role     string
    IsActive bool
}
```

- `UserID` bukan uuid atau tidak ada: `Found=false, err=nil`.
- User nonaktif: `Found=true, IsActive=false`. Konsumen (T03) yang memutuskan.

### 5.3 `lib/contracts/core/auth/finddevelopers/finddevelopers.go`

```go
package finddevelopers
const DefaultLimit = 10
const MaxLimit = 50
type Request struct{ Query string; Limit int }
type Developer struct{ ID, Name, Email string }
type Response struct{ Items []Developer } // tidak pernah nil; kosong = []Developer{}
```

- Hanya `role='developer' AND is_active`.
- `Query` di-trim; bila kosong, `Items` kosong tanpa query DB.
- `Limit <= 0` menjadi 10; `Limit > 50` menjadi 50 (di-clamp, bukan error).
- Urutan `name ASC, id ASC`.

### 5.4 `lib/contracts/core/auth/createadmin/createadmin.go`

```go
package createadmin
type Request struct{ Email, Name, Password string }
type Response struct{ UserID string }
```

File kontrak hanya berisi tipe (stdlib). Handler provider mengembalikan `*errs.Error` (`lib/common/errs`). Consumer membacanya dengan `errors.As`, tanpa meng-import package provider.

| Kasus | Error |
|---|---|
| Email sudah ada | `CONFLICT "email sudah terdaftar"` |
| Validasi gagal | `INVALID_ARGUMENT` + `details.fields` |
| Kegagalan infra | error terbungkus |

`cmd/cli` memakai `errors.As(err, &e *errs.Error)` lalu mencetak hanya `e.Message` (plus `fields` bila ada), bukan `err.Error()` yang memuat kode dan penyebab internal.

### 5.5 Konsumsi `core.audit.record` (provider T02)

Kontrak `lib/contracts/core/audit/record/record.go` mengikuti kolom `audit_log` T02:

```go
package record
const (
    ActorUser = "user"; ActorAPIKey = "api_key"; ActorAnonymous = "anonymous"; ActorSystem = "system"
    StatusOK = "ok"; StatusError = "error"; StatusDenied = "denied"
)
type Request struct {
    OccurredAt   time.Time      // zero -> diisi server
    ActorType    string
    ActorID      string         // "" -> NULL
    ActorLabel   string
    ProjectID    string         // "" -> NULL (T01 selalu kosong)
    ProjectLabel string
    Action       string
    TargetType   string
    Target       string
    Args         map[string]any
    Status       string
    Result       map[string]any
    IP           string
    UserAgent    string
    TraceID      string
}
type Response struct{ Recorded bool }
```

Koordinasi:
- Pemilik kontrak adalah T02. Tiket yang merge lebih dulu membuat konstanta dan file kontrak persis seperti di atas; tiket kedua memakainya tanpa menduplikasi.
- Bila T02 sudah merge dengan bentuk berbeda, T01 menyesuaikan diri ke T02. Hanya `base/audit` yang terdampak.

`base/audit/audit.go` (package `audit`):

```go
type Recorder interface{ Record(ctx context.Context, ev Event) }
type Event struct {
    Action     string
    TargetType string         // "user" | "session"
    Target     string         // user id target
    Args       map[string]any // TIDAK pernah berisi password/token/hash
    Status     string         // ok | denied | error
    Result     map[string]any
    Meta       base.RequestMeta
    Actor      *Actor          // nil -> dari reqctx.IdentityFrom(ctx); bila tidak ada -> anonymous
}
type Actor struct{ Type, ID, Label string }
func New(b *bus.Registry, log *slog.Logger) *BusRecorder
// Record: bangun record.Request (TraceID dari reqctx.TraceID), lalu
//   bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
//   error (termasuk ErrNotRegistered) -> log.ErrorContext(ctx, "audit record gagal", "action", ..., "error", err)
//   TIDAK mengembalikan error (fail-open untuk aksi manusia; keputusan T02).
// Defensif: kunci password/token/secret/hash di Args dihapus sebelum kirim (lapisan kedua di atas T02).
```

- Audit dipanggil usecase SETELAH transaksi commit, sehingga satu aksi menghasilkan tepat satu entri.
- Ctx pemanggil dipakai apa adanya. Bila ctx sudah dibatalkan, Call bisa gagal; kegagalan itu hanya di-log.

Katalog event T01:

| Action | Usecase | Actor | Target | Args | Status |
|---|---|---|---|---|---|
| `auth.login.success` | login | user (yang login) | user/id | `{email}` | ok |
| `auth.login.failed` | login | user bila email dikenal, selain itu anonymous dengan label email yang dicoba | user/id atau kosong | `{email_attempted, reason: invalid_credentials\|locked\|inactive}` | denied |
| `auth.logout` | logout | user | user/id | `{}` | ok |
| `auth.password.changed` | changepassword | user | user/id | `{revoked_sessions}` | ok |
| `auth.password.reset` | resetpassword | super_admin | user/id | `{generated: bool, revoked_sessions}` | ok |
| `user.created` | createuser, createadmin | super_admin / system `cli` | user/id | `{email, name, role, generated_password: bool}` | ok |
| `user.updated` | updateuser | super_admin | user/id | `{changed_fields, role_from?, role_to?}` | ok |
| `user.deactivated` | setuserstatus | super_admin | user/id | `{revoked_sessions}` | ok |
| `user.activated` | setuserstatus | super_admin | user/id | `{}` | ok |

Untuk `auth.login.failed` dengan email yang dikenal, `actor_type` diisi `user` dengan id user. Ini hanya terlihat oleh super_admin di audit log, jadi tidak membocorkan apa pun ke klien.

### 5.6 Katalog README

Tambahkan baris berikut di `docs/tickets/README.md`:

```
| `core.auth.create-admin` | Call | T01 | cmd/cli |
```

Ubah catatan `resolve-session`: handler sudah diimplementasi T01. Ubah baris `core.audit.record`: T01 memakai mode Call.

---

## 6. Migration

Satu file: `backend/migrations/core/20261021090000000_create_auth_tables.sql`

```sql
CREATE EXTENSION IF NOT EXISTS citext;
-- gen_random_uuid() bawaan Postgres >= 13 (dev/CI memakai postgres:16); tanpa pgcrypto.

CREATE TABLE users (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email                citext      NOT NULL,
    name                 text        NOT NULL,
    password_hash        text        NOT NULL,
    role                 text        NOT NULL,
    is_active            boolean     NOT NULL DEFAULT true,
    must_change_password boolean     NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL DEFAULT now(),
    created_by           uuid        NULL,
    created_by_label     text        NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    last_login_at        timestamptz NULL,
    CONSTRAINT users_email_key UNIQUE (email),
    CONSTRAINT users_role_check CHECK (role IN ('super_admin','developer')),
    CONSTRAINT users_name_len CHECK (char_length(name) BETWEEN 1 AND 100)
);
CREATE INDEX users_role_active_idx ON users (role, is_active);

CREATE TABLE sessions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    ip           inet        NULL,
    user_agent   text        NULL,
    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash)
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_last_seen_at_idx ON sessions (last_seen_at);

CREATE TABLE login_throttles (
    email             citext      PRIMARY KEY,
    failed_count      integer     NOT NULL DEFAULT 0,
    window_started_at timestamptz NOT NULL,
    locked_until      timestamptz NULL,
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_throttles_window_idx ON login_throttles (window_started_at);
```

Catatan:
- FK `sessions.user_id -> users` ada di dalam module yang sama, jadi diizinkan. Tidak ada FK ke tabel module lain (rule 8.5).
- `created_by` sengaja tanpa FK. Baris user tidak pernah dihapus aplikasi, dan label snapshot sudah disimpan (rule 8.7).
- `CREATE EXTENSION citext` butuh hak pembuatan ekstensi pada database. Dicatat di `docs/configuration.md` bagian Migrasi: role DB aplikasi butuh hak itu, atau DBA menjalankannya lebih dulu.
- Forward-only. Tidak ada `CONCURRENTLY` karena runner T00 membungkus setiap file dalam transaksi.

---

## 7. Konfigurasi baru (`infra/config`)

Dikerjakan di langkah F1 (foundation module), bukan di usecase. Wajib memperbarui tabel `docs/configuration.md` dan `.env.example`; `TestT00_AC7` dan test envexample T00 akan gagal bila tidak.

| Env | Field `Config` | Tipe | Default | Validasi |
|---|---|---|---|---|
| `SESSION_IDLE_TIMEOUT` | `SessionIdleTimeout` | duration | `12h` | > 0 dan <= absolute |
| `SESSION_ABSOLUTE_TIMEOUT` | `SessionAbsoluteTimeout` | duration | `168h` | > 0 |
| `PASSWORD_MIN_LENGTH` | `PasswordMinLength` | int | `10` | 8..128 |
| `ARGON2_MEMORY_KIB` | `Argon2MemoryKiB` | int (uint32) | `65536` | 8192..4194304 |
| `ARGON2_ITERATIONS` | `Argon2Iterations` | int (uint32) | `3` | 1..20 |
| `ARGON2_PARALLELISM` | `Argon2Parallelism` | int (uint8) | `2` | 1..16 |
| `LOGIN_MAX_FAILURES` | `LoginMaxFailures` | int | `5` | 1..100 |
| `LOGIN_FAILURE_WINDOW` | `LoginFailureWindow` | duration | `15m` | > 0 |
| `LOGIN_LOCK_DURATION` | `LoginLockDuration` | duration | `15m` | > 0 |
| `LOGIN_RATE_PER_MINUTE` | `LoginRatePerMinute` | int | `20` | 1..1000 |

- Validasi digabung dengan error lain lewat `ValidationError` (pola T00).
- Test baru di `config_test.go`: tambahkan kasus default dan kasus invalid (idle > absolute, min length 7, parallelism 0).
- Test unit auth memakai `Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}` yang dibuat langsung tanpa config, agar test cepat. Validasi config tidak berlaku di sana.

---

## 8. Plan (langkah berurutan)

```
F1 foundation module (config, migrasi, base/, base/store, base/audit, kontrak+konstanta, passwordgate, errs code, auth.yml)
 ├── U1 resolvesession ─┐
 ├── U2 getuserbus      │ jalur kritis (gelombang A, paralel)
 ├── U3 createadmin     │
 └── U4 login ──────────┘
      └── gelombang B (paralel): U5 logout, U6 getme, U7 changepassword
           └── gelombang C (paralel): U8 createuser, U9 listusers, U10 getuser, U11 updateuser,
                                      U12 setuserstatus, U13 resetpassword, U14 finddevelopers, U15 purgesessions
W  wiring final: auth/base.go (15 Init), core/base.go, cmd/cli, README/tiket, api-bundle, e2e
```

Feature hanya saling bergantung lewat `base`, sehingga gelombang B dan C secara kode bisa dikerjakan paralel setelah F1. Urutan gelombang dibuat agar e2e jalur kritis (create-admin, login, me) bisa diuji lebih awal. Setiap usecase menambahkan baris `<feature>.Init(d, shared)` di `auth/base.go` di PR-nya sendiri. Satu baris per feature, sehingga konflik merge minimal.

### F1 — foundation module

1. **Config**: tambah 10 field (bagian 7) di `internal/infra/config/config.go` beserta default, validasi, dan test. Perbarui `docs/configuration.md` (tabel dan contoh `.env`) dan `.env.example`.
2. **errs**: tambah `CodePasswordChangeRequired Code = "PASSWORD_CHANGE_REQUIRED"` di `lib/common/errs/errs.go`. Tambah ke peta bawaan `codeStatus` di `httpx/status.go` (403). Perbarui test peta.
3. **Konstanta**:
   - tambah konstanta di `lib/constants/bus.go` (5.1);
   - buat `lib/constants/auth.go` (`PasswordGateExempt`, 3.6) dan `lib/constants/auth_test.go` (semua kunci berformat `^(GET|POST|PATCH|PUT|DELETE) /api/\S+$`).
4. **Kontrak**:
   - `lib/contracts/core/auth/{getuser,finddevelopers,createadmin}/*.go`;
   - `lib/contracts/core/audit/record/record.go` bila belum dibuat T02.
   - Hanya stdlib (rule 4.5).
5. **Passwordgate**: `httpx/passwordgate.go` dan `passwordgate_test.go`. Pasang di `NewRouter` setelah `Session`. Tambah satu kasus di `router_test.go` untuk urutan middleware.
6. **Migrasi**: `backend/migrations/core/20261021090000000_create_auth_tables.sql` (bagian 6). Hapus `migrations/core/.keep` bila runner/embed tidak lagi membutuhkannya (runner mengabaikan `.keep`, jadi boleh tetap ada).
7. **base/** (package `base`): seluruh file bagian 4 beserta unit test.
8. **base/store** (package `store`):

```go
type Store struct{ q db.Querier }
func New(q db.Querier) *Store
func (s *Store) WithTx(tx *sqlx.Tx) *Store
func (s *Store) FindUserByID(ctx, id string) (base.User, bool, error)        // id bukan uuid -> (zero,false,nil) tanpa query
func (s *Store) FindUserByEmail(ctx, email string) (base.User, bool, error)
func (s *Store) LockUserByID(ctx, id string) (base.User, bool, error)        // FOR UPDATE; wajib dalam tx
func (s *Store) LockActiveSuperAdminIDs(ctx) ([]string, error)               // FOR UPDATE; wajib dalam tx
func (s *Store) InsertUser(ctx, u base.User) (base.User, error)              // RETURNING *; 23505 users_email_key -> base.ErrEmailTaken
func (s *Store) UpdatePassword(ctx, id, hash string, mustChange bool, now time.Time) error
func (s *Store) DeleteSessionsByUser(ctx, userID string, exceptTokenHash []byte) (int64, error) // nil -> semua
```

   Kolom dipetakan lewat struct row privat dengan tag `db`. `base.User` tidak diberi tag `db`, supaya entity bebas detail SQL.
   Feature hanya menaruh di `repository.go` miliknya query yang dipakai feature itu saja (`ListUsers`, `FindActiveDevelopers`, `InsertSession`, `ResolveSession`, `TouchSession`, throttle, purge, `UpdateUserFields`, `SetActive`, `DeleteSessionByHash`). Repository feature membungkus `*store.Store` untuk query bersama. `BeginTx` dipanggil dari repository feature (`func (r *Repo) InTx(ctx, fn func(*Repo) error) error`), bukan dari usecase.

9. **base/audit** (package `audit`): bagian 5.5 beserta test dengan bus nyata (`bus.NewRegistry`). Kasus yang diuji: handler palsu menerima request yang benar; endpoint tidak terdaftar menulis log error tanpa panic; kunci `password` di Args dibuang.
10. **auth/base.go** (kerangka):

```go
func Register(d app.Deps) {
    registerCodes() // sync.Once
    shared := buildShared(d.Config) // Argon2Hasher, DummyHash, Clock, Rand, policies
    // <feature>.Init(d, shared) ditambah oleh tiap usecase
}
```

   `core/base.go`: `func Register(d app.Deps) { auth.Register(d) }`. T02 kemudian menambah `auditlog.Register(d)` di baris berikutnya.
11. **OpenAPI**: `api/modules/core/auth.yml` dengan semua path pada bagian 1 dan schema pada bagian 13. Tambah `$ref` path ke `api/root.yml`, jalankan `make api-bundle`, dan pastikan `make api-check` lulus. Ini dikerjakan SEBELUM kode handler (rule 11.6).
12. **Dokumen**:
    - `docs/tickets/T01-core-auth.md`: Model data (D1), kalimat mode audit (D2), dan catatan tabel `login_throttles`;
    - `docs/tickets/README.md`: katalog bus (5.6).
13. `go.mod`: tambah `golang.org/x/crypto`.

### U1–U15

Rincian ada di plan per usecase (bagian 15). Setiap usecase:
- menambahkan path/schema di `auth.yml` bila belum ada di F1 (seharusnya sudah lengkap);
- membuat folder feature (`init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go`, test);
- menambah `<feature>.Init(d, shared)` di `auth/base.go`;
- menulis test AC yang menjadi tanggung jawabnya (bagian 10).

### W — wiring final dan CLI

1. `cmd/internal/server`: tambah helper baru (tidak mengubah yang lama):

```go
// WithDomains merakit db, bus, jobs (tidak di-Start), router dummy, Deps, dan Register semua domain,
// menjalankan fn, lalu bus.Shutdown + db.Close. Tanpa listen HTTP. Dipakai CLI.
func WithDomains(ctx context.Context, cfg config.Config, log *slog.Logger, fn func(app.Deps) error) error
```

2. `cmd/cli/cli.go` `runCreateAdmin`:
   - `server.LoadConfig(os.Getenv, stderr)`, lalu `server.WithDomains`;
   - `bus.CallTyped[createadmin.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN, createadmin.Request{...})`;
   - sukses: cetak `admin <email> dibuat (id <uuid>)` ke stdout, exit 0;
   - `*errs.Error`: cetak `Message` (dan `fields` bila ada) ke stderr, exit 1;
   - error `42P01` (tabel belum ada): pesan `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`, exit 1.

   Hapus komentar `TODO(T01)`. Ganti test `TestCreateAdmin_NotAvailableUntilT01` menjadi test integrasi T01-AC1.
3. Verifikasi `auth/base.go` memanggil ke-15 `Init` dan bahwa server menyala dengan semua route (`TestRoutesRegistered`: `chi.Walk` atas `d.API` memuat 11 route bagian 1).
4. `make api-check`, `go vet`, archtest `TestT00_AC4_RepoIsClean` lulus.

---

## 9. Changes (per file/folder)

Infra dan lib (F1):
- `backend/internal/infra/config/config.go`, `config_test.go` (ubah): 10 field baru.
- `backend/internal/infra/<httpx>/status.go` (ubah): `PASSWORD_CHANGE_REQUIRED -> 403`.
- `backend/internal/infra/<httpx>/router.go` (ubah): pasang `PasswordChangeGate` setelah `Session`.
- `backend/internal/infra/<httpx>/passwordgate.go`, `passwordgate_test.go` (baru).
- `backend/lib/common/errs/errs.go`, `errs_test.go` (ubah): `CodePasswordChangeRequired`.
- `backend/lib/constants/bus.go` (ubah), `backend/lib/constants/auth.go`, `auth_test.go` (baru).
- `backend/lib/contracts/core/auth/getuser/getuser.go` (baru).
- `backend/lib/contracts/core/auth/finddevelopers/finddevelopers.go` (baru).
- `backend/lib/contracts/core/auth/createadmin/createadmin.go` (baru).
- `backend/lib/contracts/core/audit/record/record.go` (baru bila belum ada dari T02).
- `backend/migrations/core/20261021090000000_create_auth_tables.sql` (baru).
- `backend/go.mod`, `go.sum` (ubah): `golang.org/x/crypto`.

Domain:
- `backend/internal/domain/core/base.go` (ubah satu baris): `auth.Register(d)`.
- `backend/internal/domain/core/internal/auth/base.go` (baru).
- `backend/internal/domain/core/internal/auth/base/{user,adminguard,password,session,throttle,like,errors,clock,meta,deps}.go` + `*_test.go` (baru).
- `backend/internal/domain/core/internal/auth/base/store/{store.go,store_integration_test.go}` (baru).
- `backend/internal/domain/core/internal/auth/base/audit/{audit.go,audit_test.go}` (baru).
- `backend/internal/domain/core/internal/auth/<15 feature>/...` (baru, per plan usecase).
- `backend/internal/domain/core/internal/auth/auth_integration_test.go` (baru, wiring dan AC lintas feature: AC4, AC8, AC9).

CLI:
- `backend/cmd/internal/server/run.go` atau `domains.go` (baru: `WithDomains`).
- `backend/cmd/cli/cli.go`, `main_test.go` (ubah): `runCreateAdmin`.

API dan dokumen:
- `api/modules/core/auth.yml` (baru), `api/root.yml` (ubah), `api/openapi.yml` (generated).
- `docs/configuration.md`, `.env.example` (ubah).
- `docs/tickets/T01-core-auth.md`, `docs/tickets/README.md` (ubah).

Frontend: lihat `docs/superpowers/plans/core-auth/frontend-plan.md`. Garis besarnya:
- `frontend/src/domain/core/auth/{login,changepassword,getme,logout,users}/`;
- route `/login`, `/change-password`, `/profile`, `/admin/users` (guard `meta.roles: ['super_admin']`) di `app/routes`;
- `shared/stores/session.ts` diisi dari `GET /api/auth/me`;
- redirect ke `/change-password` bila `must_change_password`;
- `temporary_password` hanya di state komponen (rule 12.9).

---

## 10. Test Scenario

Penamaan mengikuti rule 13.4 (`TestT01_ACn_...`). Test integrasi memakai `db.TestDB(t)` dan menerapkan migrasi `migrations.FS` ke schema test. Hasher di test memakai parameter argon2 kecil.

### Unit `base/` (tanpa mock)
- `TestNormalizeEmail`: trim dan lowercase; tolak `"a"`, `"a@"`, `"Budi <b@x.id>"`, 255 byte; terima `"Budi@Tim.ID "` menjadi `budi@tim.id`.
- `TestValidateName`: kosong, spasi saja, 101 rune, karakter kontrol; 100 rune multibyte diterima.
- `TestValidRole`.
- `TestUser_ApplyUpdate`: hanya field berubah yang tercatat; email dinormalisasi sebelum dibandingkan; role invalid ditolak; error gabungan.
- `TestUser_ToResponseHasNoHash`: marshal JSON tidak memuat `password_hash` maupun nilai hash.
- `TestCheckRoleChange` dan `TestCheckDeactivate` (tabel): self, last admin, admin kedua, target nonaktif, role sama.
- `TestPasswordPolicy`: 9 rune ditolak, 10 rune diterima, multibyte dihitung per rune, 257 byte ditolak, sama dengan email (beda huruf) ditolak.
- `TestArgon2_HashVerifyRoundTrip`, `TestArgon2_FormatPHC` (regex), `TestArgon2_VerifyWrongPassword`, `TestArgon2_VerifyCorruptHash` (error), `TestArgon2_VerifyUsesParamsFromHash` (hash dibuat dengan param A, diverifikasi hasher berparam B).
- `TestGeneratePassword`: panjang 16, alfabet tanpa ambigu, 1000 hasil unik, `io.Reader` deterministik menghasilkan output deterministik.
- `TestSecret_NeverPrinted`: `fmt.Sprintf("%v %s %+v")`, `slog` JSON, dan `json.Marshal` tidak memuat nilai asli.
- `TestNewSessionToken` (43 karakter, base64url), `TestHashToken` (deterministik, beda secret beda hash).
- `TestSession_IsValid` (tabel absolut/idle di batas), `TestSession_NeedsTouch` (59s false, 60s true).
- `TestThrottle_RegisterFailure`: 5 gagal mengunci 15 menit; gagal setelah window lewat me-reset; `RetryAfter` dibulatkan ke atas; `IsLocked` false setelah `LockedUntil`.
- `TestLikePattern`: `"50%_a\\"` menjadi `"%50\%\_a\\\\%"`.

### Infra
- `TestPasswordGate_BlocksWhenFlagged`: identity flagged dan `GET /api/users` menghasilkan 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestPasswordGate_AllowsExempt`: empat route exempt lolos.
- `TestPasswordGate_AnonymousPasses` dan `TestPasswordGate_UnflaggedPasses`.
- `TestPasswordGate_MethodMatters`: `GET /api/auth/change-password` diblokir.
- `TestRouter_PasswordGateAfterSession`: bus palsu `resolve-session` mengembalikan flagged, lalu `/api/users` menghasilkan 403 lewat router lengkap.
- `TestLoad_AuthDefaults`, `TestLoad_AuthInvalid` (config).
- `TestT00_AC7_AllConfigFieldsDocumented` tetap lulus.

### Integrasi `base/store` (Postgres nyata)
- `TestStore_InsertUser_EmailCaseInsensitiveUnique`: `A@x.id` lalu `a@x.id` menghasilkan `ErrEmailTaken`.
- `TestStore_FindUserByID_InvalidUUID`: `Found=false`, tanpa error.
- `TestStore_LockActiveSuperAdminIDs_Concurrency`: dua tx paralel saling menonaktifkan; satu berhasil, satu mendapat `LAST_SUPER_ADMIN` (lewat rule).
- `TestStore_DeleteSessionsByUser_Except`.
- `TestMigration_TablesAndConstraints`: role invalid ditolak CHECK; hapus user men-cascade sessions.

### Integrasi lintas module (`auth_integration_test.go`)
Router lengkap dari `httpx.NewRouter` dan `core.Register`, bus nyata, dan handler `core.audit.record` palsu yang menyimpan request ke slice (bila T02 belum merge).
- `TestT01_AC1_CreateAdminCLI` (di `cmd/cli`): run pertama exit 0 dan baris ada dengan `role=super_admin`, `must_change_password=false`; run kedua dengan email sama (beda huruf) exit 1, stderr `email sudah terdaftar`, password tidak muncul di stdout/stderr.
- `TestT01_AC2_LoginSetsHttpOnlyCookie`: atribut `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=604800`, `Secure` sesuai config.
- `TestT01_AC2_FailureResponsesIdentical`: body (tanpa `trace_id`), status, dan header (selain `X-Request-Id`) sama untuk email tidak ada dan password salah.
- `TestT01_AC2_FailureTimingEquivalent`: hasher penghitung memastikan kedua jalur memanggil `Verify` tepat sekali. Tambahan statistik: median 20 percobaan per jalur dengan argon2 param produksi kecil (m=8192), rasio 0.5..2.0; di-skip bila `testing.Short()`.
- `TestT01_AC3_LockAfterFiveFailures`: 5 kali salah, lalu login benar menghasilkan 429 `ACCOUNT_LOCKED` dengan `retry_after_seconds` > 0. Clock dimajukan 15 menit, lalu login benar 200. Juga berlaku untuk email yang tidak terdaftar (respons 429 sama).
- `TestT01_AC4_MustChangePasswordGate`: user baru dari `createuser` login, lalu `GET /api/users` 403 `PASSWORD_CHANGE_REQUIRED` (dengan role super_admin agar bukan 403 role), `GET /api/auth/me` 200, `POST /api/auth/change-password` 204, lalu `GET /api/users` 200.
- `TestT01_AC5_DeactivateKillsSessionImmediately`: user B login; admin menonaktifkan B; request B berikutnya dengan cookie lama menghasilkan 401 dan Set-Cookie Max-Age=0; tabel sessions B kosong.
- `TestT01_AC6_CannotDeactivateSelfOrLastAdmin`:
  - Admin menonaktifkan diri sendiri, atau menurunkan role sendiri lewat PATCH: 403 `SELF_ACTION_FORBIDDEN`.
  - Last admin: actor HTTP selalu super_admin aktif, jadi lewat satu request berurutan `LAST_SUPER_ADMIN` hanya bisa tercapai pada aksi ke diri sendiri, dan itu sudah lebih dulu ditolak oleh rule self. Karena itu jalur last admin diuji dua kali:
    - (a) unit `CheckDeactivate`/`CheckRoleChange` dengan count=1;
    - (b) integrasi race: admin A dan B (hanya dua super admin aktif) saling menonaktifkan di dua goroutine dengan sesi masing-masing. Tepat satu mendapat 200 dan satu 409 `LAST_SUPER_ADMIN`; di akhir tersisa tepat satu super admin aktif.
- `TestT01_AC7_DeveloperForbiddenOnUsers`: developer memanggil 7 route `/api/users*`, semuanya 403 `FORBIDDEN`.
- `TestT01_AC8_PasswordNeverLoggedOrAudited`:
  - logger dengan buffer;
  - jalankan login (benar dan salah), createuser (dengan password dan tanpa password), resetpassword, changepassword, createadmin;
  - pastikan buffer log dan seluruh `record.Request` (Args/Result di-marshal) tidak memuat satu pun password plaintext, token cookie, hash PHC, atau prefiks `$argon2id$`.
- `TestT01_AC9_EachActionOneAuditEntry`: tabel 9 aksi; setiap aksi menghasilkan tepat satu `record.Request` dengan `Action` yang benar. Aksi no-op (update tanpa perubahan, deactivate user yang sudah nonaktif) menghasilkan 0 entri.
- `TestAudit_UnregisteredDoesNotFailRequest`: tanpa handler audit, login tetap 200 dan log berisi `audit record gagal` dengan `trace_id`.
- `TestRoutesRegistered`.

Pembagian kepemilikan AC ke usecase: AC1 `createadmin`, AC2/AC3 `login`, AC4 `changepassword`, AC5 `setuserstatus`+`resolvesession`, AC6 `setuserstatus`+`updateuser`, AC7 `listusers` (seluruh grup `/api/users`), AC8/AC9 lintas usecase di W (setiap usecase menambah barisnya di tabel test).

---

## 11. E2E Scenarios (Playwright terhadap binary)

Positif:
1. `grimoire migrate up` lalu `CREATE_ADMIN_PASSWORD=... grimoire create-admin --email admin@tim.id --name Admin` menghasilkan exit 0. Login di `/login` lalu dashboard; menu Users terlihat.
2. Admin membuat developer tanpa password; dialog menampilkan password sementara sekali, dan tombol salin berfungsi.
3. Developer login dengan password sementara, otomatis diarahkan ke `/change-password`, mengganti password, lalu masuk ke beranda. Menu Users tidak terlihat.
4. Admin mencari user (`q`), memfilter role dan status, berpindah halaman.
5. Admin mengubah nama dan role developer menjadi super_admin; developer (sesi aktif) me-refresh halaman dan menu Users muncul tanpa login ulang.
6. Admin mereset password developer; sesi developer terputus; login dengan password sementara baru kembali memaksa ganti password.
7. Logout menghapus cookie; `/api/auth/me` menjadi 401 dan UI kembali ke `/login`.
8. Admin menonaktifkan lalu mengaktifkan user; user nonaktif tidak bisa login, setelah aktif bisa login lagi.

Negatif:
1. Login dengan email tidak terdaftar dan dengan password salah: pesan UI sama, `email atau password salah`.
2. 5 kali login salah, lalu login benar menampilkan pesan "terlalu banyak percobaan, coba lagi dalam X menit".
3. Developer membuka `/admin/users` langsung: UI menampilkan NotFound; API `/api/users` 403.
4. User dengan flag ganti password memanggil `GET /api/users` (fetch manual): 403 `PASSWORD_CHANGE_REQUIRED`.
5. Admin mencoba menonaktifkan dirinya sendiri: tombol nonaktif disembunyikan atau dinonaktifkan di UI, dan API (dipanggil manual) 403 `SELF_ACTION_FORBIDDEN`.
6. Membuat user dengan email yang sudah ada (beda huruf besar/kecil): 409, pesan di form.
7. Password baru 9 karakter atau sama dengan email: 400 dengan pesan per field.
8. User dinonaktifkan saat tab masih terbuka: request berikutnya 401 dan redirect ke `/login`.
9. `create-admin` kedua dengan email sama: exit 1, `email sudah terdaftar`.
10. `POST /api/auth/login` tanpa `X-Requested-With`: 403 `CSRF_REQUIRED`.
11. Lebih dari 20 login per menit dari satu IP: 429 `RATE_LIMITED` dengan `Retry-After`.

---

## 12. Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Semua request mutasi wajib header `X-Requested-With: grimoire`.

### POST /api/auth/login
Request:
```json
{ "email": "Budi@Tim.ID", "password": "rahasia-panjang-1" }
```
200 (Set-Cookie: `grimoire_session=<token>; Path=/; Max-Age=604800; HttpOnly; Secure; SameSite=Lax`):
```json
{
  "user": {
    "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "email": "budi@tim.id",
    "name": "Budi",
    "role": "developer",
    "must_change_password": true
  }
}
```
401:
```json
{ "error": { "code": "INVALID_CREDENTIALS", "message": "email atau password salah", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 akun ditunda (header `Retry-After: 812`):
```json
{ "error": { "code": "ACCOUNT_LOCKED", "message": "terlalu banyak percobaan login, coba lagi nanti", "details": { "retry_after_seconds": 812 } }, "trace_id": "..." }
```
429 rate limit IP (dari middleware T00):
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } }, "trace_id": "..." }
```
400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "email": "wajib diisi" } } }, "trace_id": "..." }
```

### POST /api/auth/logout
Request: tanpa body. 204, `Set-Cookie: grimoire_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`. Tanpa sesi: 401 `UNAUTHORIZED`.

### GET /api/auth/me
200:
```json
{ "id": "0b7c...", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": false }
```
401:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

### POST /api/auth/change-password
Request:
```json
{ "current_password": "sementara-AbC123", "new_password": "baru-sangat-rahasia" }
```
204 (sesi saat ini tetap; sesi lain user dicabut).
400:
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": {
      "current_password": "password lama salah",
      "new_password": "minimal 10 karakter"
    } }
  },
  "trace_id": "..."
}
```
Pesan lain untuk `new_password`: `"password tidak boleh sama dengan email"`, `"password baru harus berbeda dari password lama"`, `"maksimal 256 byte"`.

### 403 password gate (endpoint non-exempt)
```json
{ "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null }, "trace_id": "..." }
```

### POST /api/users
Request (password opsional):
```json
{ "email": "sari@tim.id", "name": "Sari", "role": "developer" }
```
201 (header `Cache-Control: no-store`):
```json
{
  "user": {
    "id": "7d1e...", "email": "sari@tim.id", "name": "Sari", "role": "developer",
    "is_active": true, "must_change_password": true,
    "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c...", "created_by_label": "admin@tim.id",
    "updated_at": "2026-10-21T09:00:00Z", "last_login_at": null
  },
  "temporary_password": "Kq7mZp3RtX9aHw2N"
}
```
Bila `password` dikirim, `temporary_password` bernilai `null`.
409:
```json
{ "error": { "code": "CONFLICT", "message": "email sudah terdaftar", "details": null }, "trace_id": "..." }
```
400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus super_admin atau developer", "email": "format email tidak valid" } } }, "trace_id": "..." }
```
403 (developer):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

### GET /api/users?role=developer&status=active&q=sar&limit=20&offset=0
200:
```json
{
  "items": [
    { "id": "7d1e...", "email": "sari@tim.id", "name": "Sari", "role": "developer", "is_active": true,
      "must_change_password": false, "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c...",
      "created_by_label": "admin@tim.id", "updated_at": "2026-10-21T09:05:00Z", "last_login_at": "2026-10-21T09:10:00Z" }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```
400 (`status=gone`, `limit=500`):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "status": "harus active atau inactive", "limit": "maksimal 100" } } }, "trace_id": "..." }
```

### GET /api/users/{id}
200: objek `user` (bentuk `UserDTO`, tanpa pembungkus):
```json
{ "id": "7d1e...", "email": "sari@tim.id", "name": "Sari", "role": "developer", "is_active": true, "must_change_password": false, "created_at": "...", "created_by": "0b7c...", "created_by_label": "admin@tim.id", "updated_at": "...", "last_login_at": null }
```
404 (tidak ada atau bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null }, "trace_id": "..." }
```

### PATCH /api/users/{id}
Request (minimal satu field):
```json
{ "name": "Sari W.", "role": "super_admin" }
```
200: `UserDTO` terbaru (juga ketika tidak ada perubahan, tanpa audit).
400 body kosong `{}`:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "_": "minimal satu field harus diisi" } } }, "trace_id": "..." }
```
403 menurunkan role sendiri:
```json
{ "error": { "code": "FORBIDDEN", "message": "tidak boleh menonaktifkan atau menurunkan role akun sendiri", "details": { "reason": "SELF_ACTION_FORBIDDEN" } }, "trace_id": "..." }
```
409 super admin aktif terakhir:
```json
{ "error": { "code": "CONFLICT", "message": "minimal harus ada satu super admin aktif", "details": { "reason": "LAST_SUPER_ADMIN" } }, "trace_id": "..." }
```
409 email bentrok: `CONFLICT` `"email sudah terdaftar"`.

### POST /api/users/{id}/deactivate dan /activate
Request: tanpa body. 200: `UserDTO` terbaru (idempoten: status sudah sama juga 200, tanpa audit). Error: 403 `SELF_ACTION_FORBIDDEN`, 409 `LAST_SUPER_ADMIN`, 404.

### POST /api/users/{id}/reset-password
Request (opsional):
```json
{ "password": "" }
```
200 (header `Cache-Control: no-store`):
```json
{ "user": { "id": "7d1e...", "must_change_password": true, "...": "bentuk UserDTO lengkap" }, "temporary_password": "Hn4wTq8ZmP2kRx7A" }
```
Bila `password` diisi dan valid, `temporary_password` bernilai `null`. 400 untuk validasi `password`. 404.

### Bus (Go, bukan JSON di wire)
```go
resolvesession.Request{Token: "k3J9...43char"}
resolvesession.Response{Found: true, UserID: "0b7c...", Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: false}
resolvesession.Response{Found: false} // tidak ada / kedaluwarsa / idle / user nonaktif

getuser.Request{UserID: "7d1e..."}
getuser.Response{Found: true, ID: "7d1e...", Name: "Sari", Email: "sari@tim.id", Role: "developer", IsActive: false}
getuser.Request{UserID: "bukan-uuid"} // -> getuser.Response{Found: false}, err nil

finddevelopers.Request{Query: "sar", Limit: 0} // -> Limit 10
finddevelopers.Response{Items: []finddevelopers.Developer{{ID: "7d1e...", Name: "Sari", Email: "sari@tim.id"}}}
finddevelopers.Request{Query: "  "} // -> Items: [] tanpa query DB

createadmin.Request{Email: "admin@tim.id", Name: "Admin", Password: "<dari CLI>"}
createadmin.Response{UserID: "0b7c..."}
// email sudah ada -> *errs.Error{Code: CONFLICT, Message: "email sudah terdaftar"}

bus.Call(ctx, "core.auth.get-user", "string-salah") // -> error membungkus bus.ErrBadRequestType (rule 6.9)
```

### Contoh `record.Request` (auth.login.failed, email tak dikenal)
```go
record.Request{
    ActorType: "anonymous", ActorLabel: "tidakada@tim.id",
    Action: "auth.login.failed", TargetType: "user", Target: "",
    Args: map[string]any{"email_attempted": "tidakada@tim.id", "reason": "invalid_credentials"},
    Status: "denied", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c...",
}
```

### CLI
```
$ CREATE_ADMIN_PASSWORD='...' grimoire create-admin --email admin@tim.id --name Admin
admin admin@tim.id dibuat (id 0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d)
$ echo $?            # 0
$ CREATE_ADMIN_PASSWORD='...' grimoire create-admin --email ADMIN@tim.id --name Admin
email sudah terdaftar
$ echo $?            # 1
```

---

## 13. Skema OpenAPI (`api/modules/core/auth.yml`, ringkas)

- `components/schemas`: `LoginRequest`, `Me`, `LoginResponse {user: Me}`, `ChangePasswordRequest`, `User` (= `UserDTO`), `CreateUserRequest {email, name, role, password?}`, `UserWithTempPassword {user: User, temporary_password: string|null}`, `UserList {items, total, limit, offset}`, `UpdateUserRequest {name?, email?, role?}` (`minProperties: 1`), `ResetPasswordRequest {password?}`.
- Error memakai `$ref` ke `components/responses/*` dari `foundation.yml` (BadRequest, Unauthorized, Forbidden, NotFound, Conflict, RateLimited, Internal). Tambahkan contoh kode `INVALID_CREDENTIALS`, `ACCOUNT_LOCKED`, `PASSWORD_CHANGE_REQUIRED`, `SELF_ACTION_FORBIDDEN`/`LAST_SUPER_ADMIN` (via `details.reason`).
- Parameter `X-Requested-With` didokumentasikan sebagai header wajib pada operasi mutasi.
- `securitySchemes.sessionCookie`: `type: apiKey, in: cookie, name: grimoire_session`.

---

## 14. Risiko dan catatan

- **Waktu respons**: `Verify` argon2 (sekitar 50 sampai 150 ms dengan m=64 MiB) menyamakan waktu antar jalur. Beban memori per login 64 MiB, sehingga rate limit IP (20/menit) dan throttle email menjadi pelindung DoS. Login paralel tidak dibatasi semaphore (skala tim internal); keputusan ini ditinjau ulang bila deploy memakai container RAM kecil.
- **DummyHash** dihitung sekali saat `Register`, menambah sekitar 100 ms waktu start (termasuk di CLI). Diterima.
- **Audit sebelum T02**: selama T02 belum merge, setiap aksi menulis log error `audit record gagal` (ErrNotRegistered). Ini disengaja dan terlihat di log. Test memakai handler palsu.
- **T00 belum final**: path folder `httpx` dan `jobs` bisa berubah saat merge. Plan usecase merujuk nama simbol, bukan path folder.

---

## 15. Daftar usecase (plan rinci ditulis terpisah di `docs/superpowers/plans/core-auth/<slug>.md`)

| # | Slug | Titik masuk | Gelombang | Tabel/akses | Audit | AC |
|---|---|---|---|---|---|---|
| U1 | `resolvesession` | bus `core.auth.resolve-session` | A | sessions JOIN users (baca), touch `last_seen_at` | - | AC5 |
| U2 | `getuserbus` | bus `core.auth.get-user` | A | users (baca) | - | - |
| U3 | `createadmin` | bus `core.auth.create-admin` + `cmd/cli` `runCreateAdmin` + `server.WithDomains` | A | users (insert) | `user.created` (system `cli`) | AC1 |
| U4 | `login` | `POST /api/auth/login` | A | login_throttles, users, sessions | `auth.login.success`, `auth.login.failed` | AC2, AC3 |
| U5 | `logout` | `POST /api/auth/logout` | B | sessions (delete by hash) | `auth.logout` | - |
| U6 | `getme` | `GET /api/auth/me` | B | - (reqctx) | - | AC4 (bagian) |
| U7 | `changepassword` | `POST /api/auth/change-password` (+ pemilik password gate F1) | B | users, sessions | `auth.password.changed` | AC4 |
| U8 | `createuser` | `POST /api/users` | C | users (insert) | `user.created` | AC7 (bagian) |
| U9 | `listusers` | `GET /api/users` | C | users (baca) | - | AC7 |
| U10 | `getuser` | `GET /api/users/{id}` | C | users (baca) | - | - |
| U11 | `updateuser` | `PATCH /api/users/{id}` | C | users (FOR UPDATE) | `user.updated` | AC6 (bagian) |
| U12 | `setuserstatus` | `POST /api/users/{id}/deactivate`, `/activate` | C | users (FOR UPDATE), sessions | `user.deactivated`, `user.activated` | AC5, AC6 |
| U13 | `resetpassword` | `POST /api/users/{id}/reset-password` | C | users, sessions | `auth.password.reset` | - |
| U14 | `finddevelopers` | bus `core.auth.find-developers` | C | users (baca) | - | - |
| U15 | `purgesessions` | job `core.auth.purge-sessions` (DailyAt `03:00`) | C | sessions, login_throttles (delete) | - (log jumlah) | - |

Catatan untuk penulis plan usecase:
- Jangan memecah usecase lebih kecil secara artifisial. Satu slug sama dengan satu folder feature dan satu PR yang koheren. `setuserstatus` tetap satu folder untuk dua route.
- Query `purgesessions`:
  - `DELETE FROM sessions WHERE expires_at < $1 OR last_seen_at < $2`, dengan `$1 = now` dan `$2 = now - SESSION_IDLE_TIMEOUT`;
  - `DELETE FROM login_throttles WHERE (locked_until IS NULL OR locked_until < $1) AND window_started_at < $2`, dengan `$2 = now - LOGIN_FAILURE_WINDOW`.
  - Batas waktu dihitung di Go dari Clock, sehingga test bisa mengontrol waktu dan tidak ada konversi `time.Duration` ke `interval`. Hasilnya di-log `deleted_sessions` dan `deleted_throttles`. Job idempoten.
- `resolvesession`: tidak memakai `base/audit`; error hanya untuk kegagalan infra. Token kosong atau berukuran bukan 43 karakter menghasilkan `Found=false` tanpa query.
- Semua usecase HTTP mendapat actor dari `reqctx`; route `/api/users*` sudah dijaga `RequireRole`, jadi usecase tidak mengecek role lagi. Yang tetap dicek usecase adalah aturan entity (adminguard).
- Setiap usecase menambah barisnya di test tabel AC8/AC9 (`auth_integration_test.go`).
