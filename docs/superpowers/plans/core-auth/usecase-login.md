# T01 core/auth — Usecase `login`

Konteks bersama (keputusan D1-D7, `base/`, `base/store`, `base/audit`, config, migrasi `users`/`sessions`/`login_throttles`, kode error, `base.Shared`) ada di `overview.md` dan dikerjakan di langkah foundation F1. Dokumen ini hanya memuat usecase `login`. Tiket: `docs/tickets/T01-core-auth.md`. Bila dokumen ini bertentangan dengan `overview.md`, `overview.md` menang.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (dokumen ini: backend; UI halaman login ada di `frontend-plan.md`) |
| Domain | core |
| Module | auth |
| Feature | `login` (gelombang A, jalur kritis) |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/login/`, satu baris di `backend/internal/domain/core/internal/auth/base.go`, test lintas feature `backend/internal/domain/core/internal/auth/login_integration_test.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/auth/login` | POST | tanpa prefix versi (`info.version` 0.1.0) | publik; rate limit per IP (`LOGIN_RATE_PER_MINUTE`); password gate exempt |

Tidak ada endpoint bus yang disediakan atau dikonsumsi usecase ini selain `core.audit.record` (Call, lewat `base/audit`).

AC yang dimiliki: AC2 (cookie HttpOnly, respons dan waktu setara) dan AC3 (lockout 5 kegagalan). Kontribusi ke AC8 dan AC9 (baris `login` di tabel test lintas usecase).

## Summary Plan

`login` memverifikasi email + password, membuat sesi, dan memasang cookie `grimoire_session`. Satu usecase, tidak dipecah. Orkestrasi di `usecase.go`; semua rule (normalisasi, throttle, kebijakan sesi, hasher) sudah ada di `base/`; hanya `repository.go` yang menyentuh SQL.

Alur, mengikuti overview 3.7:

1. Normalisasi `email` (trim + lowercase) dan validasi panjang saja (tanpa validasi format). Password kosong atau > 256 byte: 400.
2. Baca `login_throttles` untuk email itu. Bila terkunci: 429 `ACCOUNT_LOCKED` + `Retry-After`, audit `auth.login.failed` reason `locked`, tanpa hasher, tanpa menaikkan counter.
3. Cari user. Pilih hash: hash user bila ada, `Shared.DummyHash` bila tidak. `Hasher.Verify` dipanggil tepat satu kali di semua jalur (menyamakan waktu).
4. Sukses hanya bila `found && ok && user.CanLogin()`. Selain itu: transaksi RegisterFailure (counter naik, bisa mengunci), respons 401 `INVALID_CREDENTIALS` generik, audit `auth.login.failed` (reason `invalid_credentials` atau `inactive`, hanya di audit).
5. Sukses: satu transaksi (hapus throttle email, insert sesi dengan HMAC token, update `last_login_at`), lalu set cookie, respons profil + `must_change_password`, audit `auth.login.success` setelah commit.

Hal yang sengaja diputuskan di plan ini (selain overview):

- `Verify` yang mengembalikan error (hash di DB rusak) diperlakukan sebagai gagal biasa (401 generik + hitung throttle) dan di-log level error tanpa hash; BUKAN 500, supaya respons tidak membedakan akun.
- Jalur terkunci tidak mencari user, sehingga audit-nya memakai actor `anonymous` dengan label email yang dicoba. Ini disengaja: menghindari query tambahan pada jalur yang sudah ditolak, dan tidak ada informasi tambahan ke klien.
- Respons sukses memakai `Cache-Control: no-store`.
- Tabel `login_throttles` bisa diisi email acak oleh penyerang. Dibatasi oleh rate limit IP, batas panjang email 254 byte, dan `purgesessions` (U15) yang menghapus baris kedaluwarsa. Dicatat sebagai risiko yang diterima.
- Pemanggilan dengan sesi yang masih ada (cookie lama) tidak dicabut; login hanya membuat sesi baru. Cookie lama yang tidak valid sudah dihapus oleh middleware Session.

Prasyarat dari foundation F1 (bila belum ada, hentikan dan selesaikan F1; jangan membuatnya dari dalam folder `login/`):

- Migrasi `20261021100000000_create_auth_tables.sql` diterapkan.
- Config: `SessionSecret`, `SessionIdleTimeout`, `SessionAbsoluteTimeout`, `PasswordMinLength`, `Argon2*`, `LoginMaxFailures`, `LoginFailureWindow`, `LoginLockDuration`, `LoginRatePerMinute`, `CookieSecure`.
- `base`: `NormalizeEmail`-nya TIDAK dipakai di login (login hanya trim+lower, lihat 3.7); yang dipakai: `User.CanLogin`, `User.ToMe` (lihat catatan di Plan langkah 1), `LoginThrottle` (+`IsLocked`, `RetryAfter`, `RegisterFailure`), `ThrottlePolicy`, `Hasher`, `Secret`, `NewSessionToken`, `HashToken`, `NewSession`, `Session.MaxAgeSeconds`, `RequestMeta`, `Clock`, `Shared`, `MeDTO`, `AccountLocked`, `ErrInvalidCreds`, `InvalidFields`, kode `CodeInvalidCredentials`/`CodeAccountLocked` yang sudah didaftarkan `registerCodes()` di `auth/base.go`.
- `base/store`: `Store.FindUserByEmail`, `Store.WithTx`.
- `base/audit`: `audit.Recorder`, `audit.Event`, `audit.Actor`, `audit.New`.
- Konstanta `constants.BUS_CORE_AUDIT_RECORD` dan kontrak `record` (dari F1 atau T02).
- Kode nyata T00 (merged) yang dipakai, perhatikan lokasinya:

| Simbol | Package | Import |
|---|---|---|
| `WriteJSON`, `WriteError`, `DecodeJSON` | `httpx` | `grimoire/internal/infra/http` |
| `Rule`, `NewLimiter`, `RateLimit`, `KeyByIP`, `SessionCookieName` | `middleware` | `grimoire/internal/infra/middleware` |
| `db.Querier`, `(*db.DB).BeginTx`, `db.TestDB` | `db` | `grimoire/internal/infra/db` |
| `reqctx.TraceID` | `reqctx` | `grimoire/lib/common/reqctx` |

  Overview menyebut semua middleware di `httpx`; di kode merged, `RateLimit`/`Rule`/`KeyByIP`/`SessionCookieName` ada di package `middleware`. Plan ini memakai lokasi di kode. Nama simbol sama.

## Plan

Semua langkah di `backend/internal/domain/core/internal/auth/login/` kecuali dinyatakan lain. Kerjakan TDD: tulis test dulu per langkah.

1. **Kebutuhan kecil di `base/` (hanya bila belum dibuat F1; jangan menambah selain ini)**
   - `base.MeDTO` dan konversi `func (u User) ToMe() MeDTO` (overview 4.1 mendefinisikan `MeDTO`; pastikan method konversinya ada, dipakai juga `getme`). Bila F1 belum menyediakannya, tambahkan di `base/user.go` beserta test `TestUser_ToMe` (tidak memuat hash). Ini perubahan `base`, bukan feature, jadi dilakukan di PR F1 atau PR pertama yang membutuhkannya.
   - `base.LoginPolicyLimits`: konstanta `MaxEmailBytes = 254` dan `MaxPasswordBytes = 256` (yang kedua sudah di overview 4.3). Bila `MaxEmailBytes` belum ada, definisikan di `base/user.go`.

2. **`params.go`** (DTO HTTP, tanpa logika bisnis)

   ```go
   type LoginRequest struct {
       Email    string      `json:"email"`
       Password base.Secret `json:"password"` // log/marshal -> "***"
   }
   type LoginResponse struct {
       User base.MeDTO `json:"user"`
   }
   ```

   Validasi bentuk (tipe JSON benar, field tak dikenal ditolak) dikerjakan `httpx.DecodeJSON`. Pengecekan panjang dan kosong ada di usecase lewat rule (lihat langkah 4), bukan di handler.

3. **`repository.go`** (satu-satunya yang menyentuh SQL; membungkus `*store.Store` untuk query bersama)

   ```go
   type Repository struct {
       db    *db.DB
       store *store.Store
   }
   func NewRepository(d *db.DB) *Repository // store.New(d)

   func (r *Repository) FindUserByEmail(ctx context.Context, email string) (base.User, bool, error)
       // delegasi ke store.FindUserByEmail

   func (r *Repository) GetThrottle(ctx context.Context, email string) (base.LoginThrottle, error)
       // SELECT email, failed_count, window_started_at, locked_until FROM login_throttles WHERE email = $1
       // tidak ada baris -> LoginThrottle{Email: email} (zero), nil. Tanpa lock.

   func (r *Repository) UpdateThrottle(ctx context.Context, email string, now time.Time,
       mutate func(*base.LoginThrottle)) (base.LoginThrottle, error)
       // BeginTx:
       //  1. INSERT INTO login_throttles (email, failed_count, window_started_at)
       //     VALUES ($1, 0, $2) ON CONFLICT (email) DO NOTHING
       //  2. SELECT ... FROM login_throttles WHERE email = $1 FOR UPDATE
       //  3. mutate(&t)            // rule entity (RegisterFailure) dari usecase; repository tidak berisi rule
       //  4. UPDATE login_throttles SET failed_count=$2, window_started_at=$3, locked_until=$4, updated_at=$5 WHERE email=$1
       // mengembalikan nilai setelah mutate.

   type NewSessionRow struct { /* alias ke base.Session; tidak ada tipe baru bila base.Session cukup */ }
   func (r *Repository) CompleteLogin(ctx context.Context, email, userID string, s base.Session, now time.Time) error
       // BeginTx, satu transaksi:
       //  DELETE FROM login_throttles WHERE email = $1
       //  INSERT INTO sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent)
       //       VALUES ($1,$2,$3,$4,$5,$6,$7,$8)      -- id dari gen_random_uuid() (DEFAULT) bila s.ID kosong
       //  UPDATE users SET last_login_at = $2 WHERE id = $1   (updated_at TIDAK diubah: login bukan perubahan data)
   ```

   Catatan SQL:
   - `ip` diisi `NULL` bila `RequestMeta.IP` kosong atau tidak lolos `net.ParseIP`; selain itu string IP (kolom `inet`). `user_agent` `NULL` bila kosong.
   - Semua query parameterized (`$n`), tanpa konkatenasi string.
   - `login_throttles.email` bertipe `citext`; kita tetap mengirim email lowercase.
   - Error dibungkus `fmt.Errorf("login: <langkah>: %w", err)`.
   - Tidak ada `ON CONFLICT` pada insert sesi: bentrok `token_hash` (peluang 2^-256) dibiarkan menjadi error 500.

4. **`usecase.go`** (orkestrasi; interface didefinisikan di sisi consumer, rule 5.6)

   ```go
   type Repository interface {
       FindUserByEmail(ctx context.Context, email string) (base.User, bool, error)
       GetThrottle(ctx context.Context, email string) (base.LoginThrottle, error)
       UpdateThrottle(ctx context.Context, email string, now time.Time, mutate func(*base.LoginThrottle)) (base.LoginThrottle, error)
       CompleteLogin(ctx context.Context, email, userID string, s base.Session, now time.Time) error
   }

   type Input struct {
       Email    string
       Password base.Secret
       Meta     base.RequestMeta
   }
   type Result struct {
       Token  string       // untuk cookie; hanya dipakai handler, tidak pernah di-log
       MaxAge int          // detik, untuk cookie
       User   base.MeDTO
   }

   type Usecase struct {
       repo     Repository
       hasher   base.Hasher
       dummy    string
       clock    base.Clock
       rand     io.Reader
       session  base.SessionConfig
       throttle base.ThrottlePolicy
       audit    audit.Recorder
       log      *slog.Logger
   }
   func NewUsecase(repo Repository, s base.Shared, rec audit.Recorder, log *slog.Logger) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error)
   ```

   Alur `Execute` (hanya orkestrasi; tidak ada rule bisnis baru):

   1. `email := strings.ToLower(strings.TrimSpace(in.Email))`. Kumpulkan `base.FieldErrors`: `email` kosong atau `len > base.MaxEmailBytes` -> `"wajib diisi"` / `"maksimal 254 byte"`; password kosong -> `"wajib diisi"`, `len > base.MaxPasswordBytes` -> `"maksimal 256 byte"`. Bila ada: `return Result{}, base.InvalidFields(fe)` (400 `INVALID_ARGUMENT`). Tidak ada audit dan tidak ada query pada jalur ini.
   2. `now := u.clock()`.
   3. `th, err := u.repo.GetThrottle(ctx, email)`; error -> `fmt.Errorf("login: baca throttle: %w", err)`.
   4. Bila `th.IsLocked(now)`: `u.audit.Record(ctx, failedEvent(email, nil, "locked", in.Meta))`; `return Result{}, base.AccountLocked(th.RetryAfter(now))`.
   5. `user, found, err := u.repo.FindUserByEmail(ctx, email)`; error -> dibungkus.
   6. `hash := u.dummy; if found { hash = user.PasswordHash }`.
   7. `ok, verr := u.hasher.Verify(hash, in.Password.Reveal())` (tepat satu panggilan). Bila `verr != nil`: `u.log.ErrorContext(ctx, "login: verifikasi hash gagal", "user_id", user.ID, "error", verr)` (tanpa hash, tanpa password) lalu `ok = false`.
   8. `if found && ok && user.CanLogin()` -> jalur sukses (langkah 9). Selain itu jalur gagal:
      - `reason := "invalid_credentials"`; bila `found && ok && !user.CanLogin()` -> `reason = "inactive"`.
      - `_, err := u.repo.UpdateThrottle(ctx, email, now, func(t *base.LoginThrottle) { t.RegisterFailure(now, u.throttle) })`; error -> dibungkus dan dikembalikan sebagai 500 (kegagalan infra tidak disamarkan sebagai 401).
      - `u.audit.Record(ctx, failedEvent(email, userPtr, reason, in.Meta))` dengan `userPtr` non-nil hanya bila `found`.
      - `return Result{}, base.ErrInvalidCreds`.
      - Percobaan yang menyebabkan counter mencapai batas tetap dijawab 401 (bukan 429); 429 baru pada percobaan berikutnya.
   9. Jalur sukses:
       - `tok, err := base.NewSessionToken(u.rand)`.
       - `sess := base.NewSession(user.ID, base.HashToken(u.session.Secret, tok), now, u.session, in.Meta)`.
       - `u.repo.CompleteLogin(ctx, email, user.ID, sess, now)`; error -> dibungkus.
       - Setelah commit: `u.audit.Record(ctx, audit.Event{Action: "auth.login.success", TargetType: "user", Target: user.ID, Args: {"email": user.Email}, Status: "ok", Meta: in.Meta, Actor: &audit.Actor{Type: "user", ID: user.ID, Label: user.Email}})`. Actor eksplisit karena `reqctx` belum berisi identitas pada request login.
       - `return Result{Token: tok, MaxAge: sess.MaxAgeSeconds(u.session), User: user.ToMe()}, nil`.

   `failedEvent(email, user, reason, meta)` (helper privat di `usecase.go`): `Action: "auth.login.failed"`, `TargetType: "user"`, `Target: user.ID` bila ada, `Args: {"email_attempted": email, "reason": reason}`, `Status: "denied"`, `Actor`: `{Type: "user", ID: user.ID, Label: email}` bila user dikenal, selain itu `{Type: "anonymous", Label: email}`.

   Larangan: usecase tidak mengimpor `net/http`, tidak membangun cookie, tidak menyentuh SQL, tidak me-log password/token/hash. `audit.Recorder.Record` fail-open (tidak mengembalikan error).

5. **`handler.go`** (HTTP saja)

   ```go
   type Handler struct {
       uc           *Usecase
       cookieSecure bool
   }
   func NewHandler(uc *Usecase, cookieSecure bool) *Handler
   func (h *Handler) Login(w http.ResponseWriter, r *http.Request)
   ```

   `Login`:
   1. `var req LoginRequest; if err := httpx.DecodeJSON(r, &req); err != nil { httpx.WriteError(w, r, err); return }`.
   2. `meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}` (`truncate` memotong pada batas rune, helper privat di `handler.go`).
   3. `res, err := h.uc.Execute(r.Context(), Input{Email: req.Email, Password: req.Password, Meta: meta})`.
   4. `err != nil`: bila `*errs.Error` dengan `Code == base.CodeAccountLocked`, ambil `details["retry_after_seconds"]` dan set header `Retry-After`; lalu `httpx.WriteError(w, r, err)`. Return.
   5. Sukses: `http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookieName, Value: res.Token, Path: "/", MaxAge: res.MaxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cookieSecure})`; `w.Header().Set("Cache-Control", "no-store")`; `httpx.WriteJSON(w, http.StatusOK, LoginResponse{User: res.User})`.

   Handler tidak berisi rule bisnis. Header `Retry-After` hanya dari details yang dihasilkan `base.AccountLocked` (helper privat `retryAfterSeconds(err) (int, bool)`, diuji).

6. **`init.go`**

   ```go
   func Init(d app.Deps, s base.Shared) {
       repo := NewRepository(d.DB)
       uc := NewUsecase(repo, s, audit.New(d.Bus, d.Log), d.Log)
       h := NewHandler(uc, s.Session.CookieSecure)

       n := s.LoginRatePerMinute
       limiter := middleware.NewLimiter(d.Ctx, middleware.Rule{
           Name:    "auth.login",
           Rate:    rate.Limit(float64(n) / 60),
           Burst:   max(1, n/4),
           KeyFunc: middleware.KeyByIP,
       })
       d.API.With(middleware.RateLimit(limiter)).Post("/auth/login", h.Login)
   }
   ```

   Path relatif terhadap `d.API` (sub-router `/api` hasil `NewRouter`). Verifikasi dengan `chi.Walk` bahwa rute akhir adalah `/api/auth/login`. Rate limit dipasang per-route, tanpa mengubah rantai global T00. Limiter dibuat sekali per `Init` (bukan per request); janitor berhenti saat `d.Ctx` selesai.

7. **`auth/base.go`**: tambahkan satu baris `login.Init(d, shared)` di `Register`. Tidak ada perubahan lain di file ini (konflik merge minimal).

8. **Verifikasi akhir**: `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, `go test -tags integration` untuk test integrasi, `go test ./internal/archtest/...` (feature `login` hanya mengimpor `base`, `base/store`, `base/audit`, infra yang diizinkan, tidak mengimpor feature lain), `make api-check` (path `/api/auth/login` sudah ada di `api/modules/core/auth.yml` dari F1; bila belum, tambahkan di PR ini dan jalankan `make api-bundle`).

## Migration

Tidak ada. Tabel `users`, `sessions`, dan `login_throttles` dibuat oleh `backend/migrations/core/20261021100000000_create_auth_tables.sql` (foundation F1). Usecase ini tidak menambah kolom atau indeks. Indeks yang dipakai: PK `login_throttles(email)`, `users_email_key`, `sessions_token_hash_key`.

## Changes

Baru, semuanya di `backend/internal/domain/core/internal/auth/login/`:

- `init.go`: rakit repository, usecase, handler; daftarkan route + rate limit.
- `handler.go`: decode, bangun `RequestMeta`, set cookie dan `Retry-After`, tulis respons.
- `params.go`: `LoginRequest`, `LoginResponse`.
- `usecase.go`: interface `Repository`, `Input`, `Result`, `Usecase.Execute`, helper `failedEvent`.
- `repository.go`: `GetThrottle`, `UpdateThrottle`, `CompleteLogin`, delegasi `FindUserByEmail`.
- `usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (lihat Test Scenario).

Diubah:

- `backend/internal/domain/core/internal/auth/base.go`: tambah `login.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/login_integration_test.go` (baru, package `auth`): AC2 dan AC3 lewat router lengkap. Ditaruh di folder module karena butuh `auth.Register` (mendaftarkan kode error dan wiring); feature tidak boleh mengimpor parent.
- `backend/internal/domain/core/internal/auth/base/user.go` (hanya bila belum ada dari F1): `ToMe()` dan `MaxEmailBytes`.
- `api/modules/core/auth.yml` dan `api/openapi.yml` (hanya bila path login belum ada dari F1).

Tidak diubah: T00 (`middleware`, `httpx`, `db`), kontrak bus, migrasi, config.

## Test Scenario

Penamaan AC mengikuti rule 13.4. Hasher test: `Argon2Hasher{Params: {MemoryKiB: 64, Iterations: 1, Parallelism: 1}}` agar cepat. Waktu dikontrol lewat `base.Clock` yang diinjeksi.

### Unit usecase (`usecase_test.go`, fake `Repository`, fake `audit.Recorder`, hasher penghitung)

Hasher penghitung membungkus `Argon2Hasher` dan menghitung panggilan `Verify` serta argumen hash-nya.

- `TestLogin_Success`: hasil berisi token 43 karakter, `MaxAge` = absolute timeout (detik), `User.MustChangePassword` mengikuti user; `CompleteLogin` dipanggil sekali dengan `TokenHash == HashToken(secret, token)` dan token plaintext TIDAK dikirim ke repo; audit `auth.login.success` tepat satu dengan actor `user`.
- `TestLogin_NormalizesEmail`: input `"  Budi@Tim.ID "` memanggil repo dengan `budi@tim.id`.
- `TestLogin_InputValidation` (tabel): email kosong, spasi saja, 255 byte, password kosong, password 257 byte -> `*errs.Error` `INVALID_ARGUMENT` dengan `details.fields` yang benar; repo, hasher, dan audit tidak dipanggil. Email berformat aneh (`"bukan-email"`) TIDAK ditolak di sini (lanjut ke 401).
- `TestLogin_UnknownEmail_UsesDummyHash`: `Verify` dipanggil tepat 1 kali dengan `Shared.DummyHash`; hasil 401 `INVALID_CREDENTIALS`; `UpdateThrottle` dipanggil; audit `auth.login.failed` actor `anonymous`, label email, reason `invalid_credentials`.
- `TestLogin_WrongPassword`: `Verify` 1 kali dengan hash user; 401; audit actor `user` + target id user.
- `TestLogin_InactiveUser_SameResponseAsWrongPassword`: password benar, `IsActive=false`: error identik (`==` ke `base.ErrInvalidCreds`), throttle dinaikkan, audit reason `inactive`, tidak ada sesi (`CompleteLogin` tidak dipanggil).
- `TestLogin_Locked`: throttle `LockedUntil = now+10m`: error `ACCOUNT_LOCKED` dengan `retry_after_seconds=600`; `Verify` 0 kali; `FindUserByEmail` 0 kali; `UpdateThrottle` 0 kali; audit reason `locked`.
- `TestLogin_LockExpired_AllowsLogin`: `LockedUntil` tepat sama dengan `now` (batas) dan satu detik lalu -> lanjut ke verifikasi dan sukses.
- `TestLogin_FailureNeverRevealsReason`: error untuk tiga kasus (email tak ada, password salah, nonaktif) sama persis (code, message, details) dan pesan tidak memuat email atau reason.
- `TestLogin_VerifyError_TreatedAsFailure`: hasher mengembalikan error: hasil 401 (bukan 500), throttle dinaikkan, log berisi `login: verifikasi hash gagal` dan TIDAK memuat hash atau password.
- `TestLogin_InfraErrors` (tabel): error dari `GetThrottle`, `FindUserByEmail`, `UpdateThrottle`, `CompleteLogin` -> error terbungkus (bukan `*errs.Error` klien -> 500), tanpa audit sukses; pada `UpdateThrottle` error, tidak ada audit dan tidak ada respons 401.
- `TestLogin_FifthFailureStill401ThenLocked`: dengan repo in-memory yang memakai `base.LoginThrottle.RegisterFailure` asli: 5 kegagalan berturut-turut semuanya 401, percobaan ke-6 (password benar) 429.
- `TestLogin_AuditNeverContainsSecrets`: marshal seluruh `audit.Event` dari semua jalur; tidak memuat password, token, atau `$argon2id$`.
- `TestLogin_AuditFailureDoesNotFailLogin`: recorder yang gagal/diam (fail-open) tidak mengubah hasil.

### Unit handler (`handler_test.go`, usecase fake lewat interface kecil atau usecase dengan fake repo)

- `TestLoginHandler_SetsCookieAttributes`: `Set-Cookie` memuat `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=604800`, nama `grimoire_session`; `Secure` mengikuti `cookieSecure` (uji true dan false); `Cache-Control: no-store`.
- `TestLoginHandler_InvalidJSON`: body bukan JSON, field tak dikenal, tipe salah -> 400 `INVALID_ARGUMENT`, tanpa cookie.
- `TestLoginHandler_LockedSetsRetryAfter`: header `Retry-After` = `retry_after_seconds` dari details dan body code `ACCOUNT_LOCKED`.
- `TestLoginHandler_FailureHasNoCookie`: 401 tanpa `Set-Cookie`.
- `TestLoginHandler_UserAgentTruncated`: UA 2000 byte diteruskan 512 byte (rune-safe, tidak memotong di tengah karakter multibyte).
- `TestLoginRequest_PasswordNotPrinted`: `fmt`, `slog` JSON, dan `json.Marshal` atas `LoginRequest` tidak memuat password asli.
- `TestRetryAfterSeconds`: error lain atau details tidak berbentuk benar -> `false`.

### Integrasi repository (`repository_integration_test.go`, `db.TestDB`, migrasi diterapkan)

- `TestRepo_GetThrottle_NoRow`: mengembalikan zero value dengan email terisi.
- `TestRepo_UpdateThrottle_CreatesAndIncrements`: panggilan pertama membuat baris dengan `failed_count=1`; kelima mengunci (`locked_until` terisi).
- `TestRepo_UpdateThrottle_ConcurrentFailures`: 10 goroutine memanggil bersamaan untuk email yang sama; `failed_count` akhir tepat 10 atau terkunci sesuai policy, tidak ada update hilang (bukti `FOR UPDATE` bekerja).
- `TestRepo_UpdateThrottle_WindowReset`: baris lama dengan `window_started_at` di luar window di-reset oleh entity dan `failed_count` kembali 1.
- `TestRepo_CompleteLogin_Atomic`: sukses menghapus throttle, menyisipkan sesi (`token_hash` = hash, bukan token), dan mengisi `last_login_at`; `updated_at` user tidak berubah.
- `TestRepo_CompleteLogin_RollbackOnFailure`: sesi dengan `user_id` yang tidak ada (FK gagal) -> error, baris throttle TIDAK terhapus dan `last_login_at` tidak berubah.
- `TestRepo_CompleteLogin_IPHandling`: IP kosong atau tidak valid -> `NULL`; IPv4 dan IPv6 valid tersimpan.
- `TestRepo_FindUserByEmail_CaseInsensitive`: `citext` menemukan user walau huruf berbeda.

### Integrasi HTTP lintas module (`auth/login_integration_test.go`, router dari `httpx.NewRouter` + `auth.Register`, bus nyata, handler `core.audit.record` palsu yang menyimpan request ke slice bila T02 belum merge; semua request mutasi membawa `X-Requested-With: grimoire`)

- `TestT01_AC2_LoginSetsHttpOnlyCookie`: login benar -> 200; cookie `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=604800`, `Secure` sesuai config; cookie dapat dipakai pada `resolve-session` (request `GET` ke route terproteksi yang ada, atau panggilan bus langsung bila `getme` belum ada); token di cookie tidak sama dengan nilai apa pun di tabel `sessions` (hanya hash).
- `TestT01_AC2_FailureResponsesIdentical`: email tidak ada, password salah, dan user nonaktif memberi status, body (setelah `trace_id` dihapus), dan header (selain `X-Request-Id`/trace/date) yang sama.
- `TestT01_AC2_FailureTimingEquivalent`: (a) deterministik: hasher penghitung memastikan `Verify` dipanggil tepat 1 kali pada email tidak ada dan pada password salah; (b) statistik: median 20 percobaan per jalur dengan argon2 `m=8192`, rasio 0.5..2.0, di-skip bila `testing.Short()`.
- `TestT01_AC3_LockAfterFiveFailures`: 5 login salah -> 401 semua; login benar ke-6 -> 429 `ACCOUNT_LOCKED` dengan `retry_after_seconds > 0`, header `Retry-After` sama, dan tidak ada sesi baru. Clock dimajukan 15 menit + 1 detik -> login benar 200 dan baris throttle terhapus.
- `TestT01_AC3_LockAppliesToUnknownEmail`: 5 gagal untuk email tak terdaftar, lalu percobaan ke-6 -> 429 dengan bentuk respons yang sama dengan email terdaftar.
- `TestT01_AC3_LockedAttemptDoesNotExtendLock`: percobaan saat terkunci tidak mengubah `locked_until`.
- `TestT01_AC3_SuccessResetsCounter`: 4 gagal, 1 sukses, lalu 4 gagal lagi -> belum terkunci.
- `TestLogin_RateLimitPerIP`: lebih dari burst dalam satu detik dari satu IP -> 429 `RATE_LIMITED` dengan `Retry-After`; IP lain tidak terpengaruh.
- `TestLogin_RequiresCSRFHeader`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
- `TestLogin_MustChangePasswordUserCanLogin`: user dengan flag tetap mendapat 200 dengan `must_change_password: true` (login exempt dari password gate).
- `TestLogin_StaleCookieIgnored`: request login dengan cookie sesi tidak valid tetap sukses dan menimpa cookie.
- Baris `login` di tabel test AC8/AC9: login benar dan salah tidak meninggalkan password/token/hash di buffer log atau argumen audit (AC8); login benar menghasilkan tepat 1 `record.Request` `auth.login.success`, login gagal tepat 1 `auth.login.failed` (AC9). `TestAudit_UnregisteredDoesNotFailRequest`: tanpa handler audit, login tetap 200 dan log memuat `audit record gagal` dengan `trace_id`.

## E2E Scenarios

Playwright terhadap binary (UI di `frontend-plan.md`; di sini hanya perilaku yang dimiliki `login`).

Positif:

1. Admin hasil `create-admin` membuka `/login`, mengisi email (huruf campuran, ada spasi di ujung) dan password benar -> masuk ke dashboard; cookie `grimoire_session` ada, `HttpOnly` (tidak terbaca `document.cookie`).
2. User dengan `must_change_password=true` login -> otomatis dialihkan ke `/change-password`.
3. Login lagi setelah logout atau setelah sesi kedaluwarsa berhasil dan membuat sesi baru.
4. Setelah 5 kegagalan, tunggu lock habis (clock/konfigurasi E2E `LOGIN_LOCK_DURATION=5s`) lalu login benar berhasil.

Negatif:

1. Email tidak terdaftar dan password salah menampilkan pesan UI yang sama: `email atau password salah`.
2. User nonaktif dengan password benar menampilkan pesan yang sama.
3. 5 kali login salah, lalu login benar menampilkan pesan terkunci `terlalu banyak percobaan login, coba lagi nanti` (+ waktu tunggu dari `retry_after_seconds`); form tidak mengungkap apakah email terdaftar.
4. Lebih dari batas per menit dari satu IP: 429 `RATE_LIMITED`, UI menampilkan pesan umum dan tombol dinonaktifkan sampai `Retry-After`.
5. `POST /api/auth/login` tanpa `X-Requested-With` (curl manual) -> 403 `CSRF_REQUIRED`.
6. Body kosong atau field kosong -> 400 dengan pesan per field; password lebih dari 256 byte -> 400.
7. Server tidak dapat menulis audit (T02 belum ada): login tetap berhasil.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib membawa header `X-Requested-With: grimoire`. Body maksimum mengikuti `HTTPDefaultBodyBytes`.

### POST /api/auth/login

Request:

```json
{ "email": "Budi@Tim.ID", "password": "rahasia-panjang-1" }
```

200 (header: `Cache-Control: no-store`, `Set-Cookie: grimoire_session=<token 43 karakter>; Path=/; Max-Age=604800; HttpOnly; Secure; SameSite=Lax`):

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

400 (validasi bentuk; contoh email dan password kosong):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "email": "wajib diisi", "password": "wajib diisi" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

400 (JSON rusak atau field tak dikenal):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "JSON tidak valid", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

401 (email tidak ada, password salah, atau user nonaktif; identik):

```json
{ "error": { "code": "INVALID_CREDENTIALS", "message": "email atau password salah", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

403 (tanpa header CSRF):

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib untuk request mutasi", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

429 akun ditunda (header `Retry-After: 812`):

```json
{
  "error": {
    "code": "ACCOUNT_LOCKED",
    "message": "terlalu banyak percobaan login, coba lagi nanti",
    "details": { "retry_after_seconds": 812 }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

429 rate limit IP (dari middleware T00, header `Retry-After: 3`):

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "terlalu banyak permintaan, coba lagi nanti",
    "details": { "retry_after_seconds": 3 }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

500 (kegagalan DB atau infra; tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

(teks `message` 500 mengikuti `respond.WriteError` T00.)

### Audit yang dihasilkan (`record.Request`, bukan respons HTTP)

Sukses:

```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "budi@tim.id",
    Action: "auth.login.success", TargetType: "user", Target: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    Args: map[string]any{"email": "budi@tim.id"},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-...",
}
```

Gagal, email tidak dikenal:

```go
record.Request{
    ActorType: "anonymous", ActorLabel: "tidakada@tim.id",
    Action: "auth.login.failed", TargetType: "user", Target: "",
    Args: map[string]any{"email_attempted": "tidakada@tim.id", "reason": "invalid_credentials"},
    Status: "denied", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-...",
}
```

Gagal, user dikenal (password salah, `reason: "invalid_credentials"`; nonaktif, `reason: "inactive"`): `ActorType: "user"`, `ActorID` dan `Target` berisi id user. Terkunci: `ActorType: "anonymous"`, `reason: "locked"`. Tidak ada kasus yang memuat password, token, atau hash.
