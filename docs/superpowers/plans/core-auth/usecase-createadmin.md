# T01 core/auth — Usecase `createadmin` (bus `core.auth.create-admin` + CLI `grimoire create-admin`)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (foundation F1 harus sudah ada: migrasi `users`, `base/` (`NormalizeEmail`, `ValidateName`, `PasswordPolicy`, `Hasher`, `Secret`, `ErrEmailTaken`, `EmailConflict`, `InvalidFields`), `base/store` (`InsertUser`), `base/audit`, `base.Shared`, kontrak `createadmin`, konstanta `BUS_CORE_AUTH_CREATE_ADMIN`). Tiket: `docs/tickets/T01-core-auth.md` (AC1). Konvensi: `docs/tickets/README.md` dan `rules.txt`. Bila dokumen ini bertentangan dengan `overview.md`, `overview.md` menang.

Catatan nama package (T00 sudah merge, kode nyata): router/server ada di `grimoire/internal/infra/http` (diimpor sebagai `httpx`), middleware di `grimoire/internal/infra/middleware`, job di `grimoire/internal/infra/jobs`, DB di `grimoire/internal/infra/db`. Nama simbol sama dengan overview; ikuti path nyata ini.

## Metadata

| Field | Nilai |
|---|---|
| Endpoint | bus `core.auth.create-admin` (konstanta `constants.BUS_CORE_AUTH_CREATE_ADMIN`); titik masuk manusia: CLI `./grimoire create-admin --email E --name N`. Tidak ada endpoint HTTP |
| Method | bus `Call` (sinkron). CLI: perintah `create-admin` |
| Version | kontrak bus baru, tanpa prefix versi (`lib/contracts/core/auth/createadmin`). Tidak ada perubahan OpenAPI |
| Scope | backend (tidak ada frontend) |
| Domain | core |
| Module | auth |
| Feature | `createadmin` (gelombang A, U3) |
| Konsumen bus | `cmd/cli` saja (rule 6: antar module hanya lewat bus; CLI adalah consumer di luar domain) |
| Audit | `user.created`, actor `system` label `cli`, status `ok` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/createadmin/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `createadmin.Init`), `backend/cmd/internal/server/` (helper `WithDomains`, file baru `domains.go`), `backend/cmd/cli/` (`cli.go`, `main_test.go`, `smoke_integration_test.go`, `createadmin_integration_test.go`), `docs/tickets/README.md` (hanya bila F1 belum menambah baris katalog) |

## Summary Plan

Operator membuat Super Admin pertama dari terminal. Alurnya:

1. `cmd/cli` sudah mem-parse flag dan mengambil password (`CREATE_ADMIN_PASSWORD`, stdin, atau prompt TTY; `resolvePassword` di `password.go`, tidak diubah).
2. `runCreateAdmin` (sekarang stub `TODO(T01)`) diganti: muat config, rakit db + bus + router + `Deps` + `core.Register` lewat helper baru `server.WithDomains`, lalu `bus.CallTyped[createadmin.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN, createadmin.Request{...})`.
3. Handler bus `createadmin` memanggil usecase: normalisasi dan validasi email, nama, dan password (kebijakan sama dengan user biasa), hash argon2id, `INSERT users` dengan `role='super_admin'`, `is_active=true`, `must_change_password=false`, `created_by=NULL`, `created_by_label='cli'`.
4. Email sudah ada (unique `users_email_key`, case-insensitive via `citext`) dipetakan ke `*errs.Error{CONFLICT, "email sudah terdaftar"}`. CLI mencetak pesan itu ke stderr dan exit 1.
5. Setelah insert sukses, usecase mengirim audit `user.created` (actor system `cli`) lewat `base/audit` (bus `Call`, fail-open), lalu CLI mencetak `admin <email> dibuat (id <uuid>)` ke stdout dan exit 0.

Keputusan desain:
- Satu usecase `Create`. Tidak dipecah. Satu statement `INSERT`, jadi tanpa transaksi; race dua proses `create-admin` bersamaan dengan email sama diselesaikan constraint unik (satu sukses, satu `CONFLICT`), bukan pengecekan lebih dulu.
- `must_change_password=false`: operator sendiri memilih password (overview 3.10). Berbeda dari `createuser`/`resetpassword`.
- Tidak ada `temporary_password` dan tidak ada password acak: password wajib. Request kosong password -> `INVALID_ARGUMENT` (`fields.password`). Tidak pernah dikembalikan di response.
- Tidak ada `RequestMeta`/IP/UserAgent; audit `IP`/`UserAgent` kosong. `TraceID` kosong (tidak ada request HTTP); `base/audit` menerima ctx tanpa trace dengan aman.
- Pesan error ke operator hanya `Message` dan `fields` dari `*errs.Error`, bukan `err.Error()` (yang memuat kode dan penyebab internal, rule overview 5.4). Error infra dicetak dengan pesan tetap; detail masuk log (stderr), tidak memuat password.
- Audit saat T02 belum merge: `core.audit.record` tidak terdaftar, `base/audit` menulis log error `audit record gagal` dan tetap sukses (fail-open, overview 14). Pesan log itu muncul di stderr CLI; ini disengaja dan diuji.
- `WithDomains` tidak menjalankan migrasi dan tidak menjalankan scheduler/listen HTTP. Operator wajib `grimoire migrate up` lebih dulu; tabel belum ada (`42P01`) menghasilkan pesan yang menunjuk ke langkah itu.

## Plan

Urutan wajib. Prasyarat F1 harus ada; bila belum, hentikan dan laporkan, jangan menambahkannya di luar allowedPaths.

1. **Cek kontrak dan konstanta F1.** Pastikan ada `lib/contracts/core/auth/createadmin/createadmin.go` (`Request{Email, Name, Password string}`, `Response{UserID string}`), `constants.BUS_CORE_AUTH_CREATE_ADMIN = "core.auth.create-admin"`, dan baris katalog `| core.auth.create-admin | Call | T01 | cmd/cli |` di `docs/tickets/README.md`. Kontrak hanya stdlib (rule 4.5). Tidak ada OpenAPI (bukan HTTP).

2. **`createadmin/repository.go`** (satu-satunya yang menyentuh SQL, lewat `base/store`):

   ```go
   package createadmin

   type Repository interface {
       InsertUser(ctx context.Context, u base.User) (base.User, error) // ErrEmailTaken bila 23505 users_email_key
   }

   type repo struct{ s *store.Store }

   func newRepo(q db.Querier) *repo { return &repo{s: store.New(q)} }
   func (r *repo) InsertUser(ctx context.Context, u base.User) (base.User, error) { return r.s.InsertUser(ctx, u) }
   ```
   Tidak ada query baru; semua SQL ada di `store.InsertUser` (F1). Wrapper tipis ini ada agar usecase bergantung pada interface lokal (rule 5.6), bukan tipe `store.Store`.

3. **`createadmin/usecase.go`** (orkestrasi saja; rule bisnis ada di `base`):

   ```go
   type Usecase struct {
       repo   Repository
       hasher base.Hasher
       audit  audit.Recorder
       policy base.PasswordPolicy
       clock  base.Clock
   }

   func (u *Usecase) Create(ctx context.Context, in createadmin.Request) (createadmin.Response, error)
   ```
   Langkah `Create`, berurutan:
   1. `email, err := base.NormalizeEmail(in.Email)`; `name, err := base.ValidateName(in.Name)`; `pw := base.Secret(in.Password)`; `u.policy.ValidateField("password", pw.Reveal(), email)`. Kumpulkan semua `FieldErrors` lalu bila ada kembalikan `base.InvalidFields(fe)` (400-setara `INVALID_ARGUMENT` dengan `details.fields`), tanpa memanggil hasher.
   2. `hash, err := u.hasher.Hash(pw.Reveal())`; gagal -> `fmt.Errorf("createadmin: hash password: %w", err)`.
   3. Bangun entity `base.User{Email: email, Name: name, PasswordHash: hash, Role: base.RoleSuperAdmin, IsActive: true, MustChangePassword: false, CreatedByLabel: ptr("cli")}` (CreatedBy nil; ID dan timestamp diisi DB lewat `RETURNING`).
   4. `created, err := u.repo.InsertUser(ctx, user)`. `errors.Is(err, base.ErrEmailTaken)` -> `return Response{}, base.EmailConflict()`. Error lain -> `fmt.Errorf("createadmin: insert user: %w", err)`.
   5. `u.audit.Record(ctx, audit.Event{Action: "user.created", TargetType: "user", Target: created.ID, Args: map[string]any{"email": created.Email, "name": created.Name, "role": created.Role, "generated_password": false}, Status: record.StatusOK, Actor: &audit.Actor{Type: record.ActorSystem, Label: "cli"}})`. Dipanggil SETELAH insert sukses; `Record` tidak mengembalikan error (fail-open).
   6. `return createadmin.Response{UserID: created.ID}, nil`.

   Argumen audit TIDAK berisi password, hash, atau token. Usecase tidak mengimpor `net/http` dan tidak mengenal bus (rule 5.2).

4. **`createadmin/handler.go`** (adaptor bus, rule 6.9):

   ```go
   func (h *handler) handle(ctx context.Context, req any) (any, error) {
       in, err := bus.Expect[createadmin.Request](req)
       if err != nil { return nil, err }          // membungkus bus.ErrBadRequestType
       return h.uc.Create(ctx, in)                // Response (nilai, bukan pointer) atau *errs.Error / error terbungkus
   }
   ```
   Handler mengembalikan `createadmin.Response` bernilai (bukan pointer) supaya `bus.CallTyped[createadmin.Response]` berhasil. Handler tidak mencatat (log) `req`, karena `req.Password` ada di dalamnya. Tidak ada `params.go` (bukan HTTP).

5. **`createadmin/init.go`**:

   ```go
   func Init(d app.Deps, s base.Shared) {
       uc := &Usecase{
           repo:   newRepo(d.DB),
           hasher: s.Hasher,
           audit:  audit.New(d.Bus, d.Log),
           policy: s.Password,
           clock:  s.Clock,
       }
       d.Bus.Register(constants.BUS_CORE_AUTH_CREATE_ADMIN, (&handler{uc: uc}).handle)
   }
   ```
   `bus.Register` panic bila endpoint ganda; ini disengaja (satu provider). Bila `clock` tidak terpakai usecase, hapus fieldnya (jangan menambah dependensi tanpa kegunaan).

6. **`auth/base.go`**: tambahkan satu baris `createadmin.Init(d, shared)` di `Register` (satu baris per feature, konflik merge minimal).

7. **`cmd/internal/server/domains.go`** (file baru; `Run` dan helper lama TIDAK diubah):

   ```go
   // WithDomains merakit db, bus, router dummy, Deps, dan Register semua domain tanpa listen HTTP
   // dan tanpa Start scheduler, menjalankan fn, lalu bus.Shutdown + db.Close. Dipakai CLI.
   func WithDomains(ctx context.Context, cfg config.Config, log *slog.Logger, fn func(app.Deps) error) error
   ```
   Isi, mengikuti rakitan `Run` (baris 55 sampai 80) tetapi tanpa migrasi:
   1. `d, err := db.Open(ctx, cfg)`; gagal -> `fmt.Errorf("open db: %w", err)`.
   2. `b := bus.NewRegistry(log)`; `sched := jobs.New(d, log)` (tidak di-`Start`, jadi `purgesessions` terdaftar tetapi tidak jalan).
   3. `srvCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))`; `_, api, mcp := httpx.NewRouter(cfg, log, b, d)`; `deps := app.Deps{Ctx: srvCtx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: sched, API: api, MCP: mcp}`.
   4. `core.Register(deps)`, `project.Register(deps)`, `discovery.Register(deps)` (urutan sama dengan `Run`).
   5. `ferr := fn(deps)`.
   6. Teardown selalu dijalankan: `cancel()`, `b.Shutdown(ctx dengan cfg.BusShutdownTimeout)` (menunggu audit Publish lain selesai), `d.Close()`. Gabungkan error teardown dengan `errors.Join(ferr, ...)`; `ferr` tetap bisa dibaca `errors.As` oleh pemanggil.
   7. Tidak mencatat `cfg` ke log (berisi secret). `Run` mencatat `"config", cfg` lewat `slog`; CLI tidak meniru itu.

8. **`cmd/cli/cli.go`**: hapus komentar `TODO(T01)` dan stub. Ganti dengan:

   - Field baru di struct `cli`: `createAdminRun func(ctx context.Context, a createAdminArgs) error`, diisi di `newCLI` dengan implementasi default `c.callCreateAdmin`. Pemanggil di `createAdmin(...)` berubah menjadi `if err := c.createAdminRun(ctx, args); err != nil { c.reportCreateAdminError(err); return 1 }`. Hook ini memungkinkan test unit tanpa DB (pola sama dengan `serve`/`migrateUp`).
   - `callCreateAdmin`:
     1. `cfg, log, ok := server.LoadConfig(c.getenv, c.stderr)`; `!ok` -> kembalikan `errAlreadyReported` (sentinel; `reportCreateAdminError` tidak mencetak ulang, exit 1). Config tidak valid sudah mencetak pesan (tanpa secret).
     2. `ctx, cancel := context.WithTimeout(ctx, 30*time.Second)` (argon2 + DB; mencegah menggantung).
     3. `err := server.WithDomains(ctx, cfg, log, func(d app.Deps) error { res, err := bus.CallTyped[createadmin.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN, createadmin.Request{Email: a.Email, Name: a.Name, Password: a.Password}); if err != nil { return err }; fmt.Fprintf(c.stdout, "admin %s dibuat (id %s)\n", a.Email, res.UserID); return nil })`.
        Email yang dicetak adalah `a.Email` dari flag (CLI sudah memvalidasi format; usecase menormalkan ke lowercase di DB). Bila ingin menampilkan versi tersimpan, tambahkan `Email` ke `Response`; TIDAK dilakukan di sini (kontrak overview 5.4 hanya `UserID`).
   - `reportCreateAdminError(err error)`, fungsi murni yang bisa diuji:
     - `errors.Is(err, errAlreadyReported)`: tidak mencetak apa pun.
     - `var e *errs.Error; errors.As(err, &e)`: cetak `e.Message` ke stderr; bila `e.Details` adalah `map[string]any` berisi `fields` (`base.FieldErrors`/`map[string]string`), cetak satu baris per field terurut abjad: `  password: minimal 10 karakter`. Jangan cetak `e.Code`, `e.Err`, atau `err.Error()`.
     - `var pgErr *pgconn.PgError; errors.As(err, &pgErr) && pgErr.Code == "42P01"`: cetak `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`.
     - selain itu: cetak `gagal membuat admin; periksa log`, dan `log.Error("create-admin gagal", "error", err)` dilakukan di `callCreateAdmin` (bukan di sini). Isi error tidak boleh memuat password; password hanya hidup di `createAdminArgs` dan `createadmin.Request`, tidak pernah di-wrap ke error mana pun.
   - Tambahkan import `grimoire/lib/constants`, `grimoire/lib/contracts/core/auth/createadmin`, `grimoire/lib/common/errs`, `grimoire/internal/infra/bus`, `grimoire/internal/infra/app`, `github.com/jackc/pgx/v5/pgconn`. Pastikan archtest T00 (`TestT00_AC4_RepoIsClean`) mengizinkan `cmd/cli` mengimpor `lib/*` dan `infra/*` (tidak boleh mengimpor `internal/domain/.../internal/...`).
   - Perbarui `usage` hanya jika perlu; teks `create-admin --email E --name N  buat akun super admin pertama` sudah benar. Tambahkan ke baris "Env wajib" tidak diperlukan.
   - Logger (sudah diverifikasi di kode T00): `server.LoadConfig` mengembalikan `logger.New(cfg)` yang menulis ke `os.Stdout`. Stdout CLI harus bersih (hanya pesan sukses), jadi di `callCreateAdmin` abaikan logger dari `LoadConfig` dan buat `log := logger.NewWithWriter(cfg, c.stderr)` (`grimoire/internal/infra/logger`, sudah ada). Jangan mengubah paket `logger` (infra, di luar allowedPaths).

9. **Perbarui test lama yang mengasumsikan stub** (lihat Changes): ganti `TestCreateAdmin_NotAvailableUntilT01` dan kasus pertama `TestBuiltBinary_CreateAdminNotAvailable`.

10. **Verifikasi akhir:** `go vet ./...`, `go test ./internal/domain/core/internal/auth/createadmin/... ./cmd/...`, test integrasi (butuh `TEST_DATABASE_URL`), archtest, lalu uji manual: `grimoire migrate up` lalu `CREATE_ADMIN_PASSWORD=... grimoire create-admin --email admin@tim.id --name Admin` dua kali.

## Migration

Tidak ada migrasi baru. Memakai tabel `users` dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (F1). Kolom yang ditulis: `email` (citext, unique `users_email_key`), `name`, `password_hash`, `role='super_admin'`, `is_active=true`, `must_change_password=false`, `created_by=NULL`, `created_by_label='cli'`. Prasyarat runtime: operator sudah menjalankan `grimoire migrate up` (atau `RUN_MIGRATIONS_ON_START=true` pada `serve` sebelumnya).

## Changes

Baru:
- `backend/internal/domain/core/internal/auth/createadmin/init.go`
- `backend/internal/domain/core/internal/auth/createadmin/handler.go`
- `backend/internal/domain/core/internal/auth/createadmin/usecase.go`
- `backend/internal/domain/core/internal/auth/createadmin/repository.go`
- `backend/internal/domain/core/internal/auth/createadmin/usecase_test.go` (unit, fake repo, fake auditor, hasher cepat)
- `backend/internal/domain/core/internal/auth/createadmin/handler_test.go` (bus nyata, `bus.NewRegistry`)
- `backend/internal/domain/core/internal/auth/createadmin/integration_test.go` (Postgres nyata, `db.TestDB`)
- `backend/cmd/internal/server/domains.go` dan `domains_test.go` (`WithDomains`)
- `backend/cmd/cli/createadmin_integration_test.go` (AC1 end-to-end binary)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `createadmin.Init(d, shared)`.
- `backend/cmd/cli/cli.go`: hapus `TODO(T01)` dan stub `runCreateAdmin`; tambah field `createAdminRun`, `callCreateAdmin`, `reportCreateAdminError`, sentinel `errAlreadyReported`.
- `backend/cmd/cli/main_test.go`: hapus `TestCreateAdmin_NotAvailableUntilT01`; tambah test CLI berbasis hook (lihat Test Scenario). Helper `newTestCLI` perlu mengizinkan menimpa `createAdminRun`.
- `backend/cmd/cli/smoke_integration_test.go`: `TestBuiltBinary_CreateAdminNotAvailable` -> ubah nama menjadi `TestBuiltBinary_CreateAdminInputValidation`; kasus pertama (yang mengharapkan teks `belum tersedia`) diganti: tanpa `DATABASE_URL`/`SESSION_SECRET` dengan `CREATE_ADMIN_PASSWORD=rahasia123` -> exit 1, stderr memuat `DATABASE_URL`, tidak memuat `rahasia123`. Dua kasus lain (exit 2) tetap.
- `docs/tickets/README.md`: hanya bila baris katalog `core.auth.create-admin` belum ditambahkan oleh F1.

Tidak diubah (jangan sentuh): `cmd/internal/server/run.go`, `cmd/cli/password.go`, `internal/infra/*`, `lib/*` (kontrak/konstanta milik F1), usecase lain.

## Test Scenario

Penamaan AC mengikuti rule 13.4 (`TestT01_AC1_...`). Hasher di test unit/integrasi memakai `Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}`.

### Unit `createadmin/usecase_test.go` (tanpa DB)
- `TestCreate_Success`: repo palsu menyimpan entity; ditegaskan `Role=super_admin`, `IsActive=true`, `MustChangePassword=false`, `CreatedBy=nil`, `CreatedByLabel="cli"`, email lowercase dan ter-trim (`" Admin@Tim.ID "` -> `admin@tim.id`), hash berawalan `$argon2id$` dan BUKAN sama dengan password; response `UserID` sama dengan id dari repo.
- `TestCreate_EmailTaken_ReturnsConflict`: repo mengembalikan `base.ErrEmailTaken` -> `errors.As` ke `*errs.Error` dengan `Code=CONFLICT`, `Message="email sudah terdaftar"`; auditor TIDAK dipanggil.
- `TestCreate_ValidationErrors` (tabel): email `"a@"`, nama kosong, nama 101 rune, password 9 rune, password sama dengan email (beda huruf), password 257 byte. Masing-masing -> `INVALID_ARGUMENT` dengan `details.fields` yang memuat kunci yang benar; repo dan hasher tidak dipanggil. Satu kasus gabungan (email dan password salah sekaligus) -> kedua field ada.
- `TestCreate_InfraErrorWrapped`: repo mengembalikan error acak -> error terbungkus `createadmin: insert user:`, `errors.Is` ke penyebab, BUKAN `*errs.Error`; auditor tidak dipanggil. Error hasher -> terbungkus `createadmin: hash password:`.
- `TestCreate_AuditEvent`: tepat satu `Record` dengan `Action="user.created"`, `TargetType="user"`, `Target=<id>`, `Status="ok"`, `Actor={Type:"system", ID:"", Label:"cli"}`, `Args` memuat `email`, `name`, `role`, `generated_password=false`.
- `TestCreate_AuditNeverContainsSecrets`: marshal `Args` dan seluruh `Event`; tidak memuat password plaintext maupun `$argon2id$`.
- `TestCreate_AuditAfterInsertOnly`: urutan panggilan fake (insert lalu audit); audit tidak dipanggil pada jalur gagal.

### Handler/bus `createadmin/handler_test.go` (bus nyata)
- `TestHandler_RegisteredAndTyped`: `Init` pada `bus.NewRegistry` dengan fake dependensi; `bus.CallTyped[createadmin.Response]` mengembalikan `UserID`.
- `TestHandler_WrongRequestType`: `bus.Call(ctx, endpoint, "string-salah")` -> error membungkus `bus.ErrBadRequestType` (rule 6.9).
- `TestHandler_ConflictVisibleThroughBus`: error `*errs.Error` dari usecase tetap terbaca lewat `errors.As` setelah melewati `bus.Call`.
- `TestHandler_DoesNotLogRequest`: logger buffer pada registry; setelah panggilan sukses dan gagal, buffer tidak memuat password.
- `TestInit_DuplicateRegistrationPanics`: `Init` dua kali pada registry sama panik (perilaku bus; mendokumentasikan bahwa `Register` tidak boleh dipanggil ganda).

### Integrasi `createadmin/integration_test.go` (Postgres via `db.TestDB`, migrasi diterapkan)
- `TestCreateAdmin_PersistsRow`: lewat bus nyata; baris `users` ada dengan `role='super_admin'`, `must_change_password=false`, `is_active=true`, `created_by IS NULL`, `created_by_label='cli'`, `password_hash` berformat PHC dan Verify(password) sukses dengan hasher yang sama.
- `TestT01_AC1_DuplicateEmailCaseInsensitive`: panggilan kedua dengan `ADMIN@TIM.ID` -> `CONFLICT`; hanya satu baris.
- `TestCreateAdmin_ConcurrentSameEmail`: 8 goroutine memanggil bersamaan dengan email sama; tepat satu sukses, tujuh `CONFLICT`, satu baris di DB (bukti constraint unik yang menjaga race).
- `TestCreateAdmin_AuditRecordedViaBus`: handler palsu `core.audit.record` menangkap `record.Request`; tepat satu dengan `ActorType="system"`, `ActorLabel="cli"`, `Action="user.created"`, `Target=<id>`; pada jalur CONFLICT: nol entri.
- `TestCreateAdmin_AuditUnregistered_StillSucceeds`: tanpa handler audit, panggilan sukses, baris ada, log memuat `audit record gagal`.

### `cmd/internal/server/domains_test.go`
- `TestWithDomains_RegistersAuthEndpointsAndTearsDown`: dengan DB test; di dalam `fn`, `bus.Call` ke `core.auth.create-admin` dengan tipe salah menghasilkan `ErrBadRequestType` (bukti handler terdaftar, bukan `ErrNotRegistered`); setelah return, `db` tertutup (query pada pool gagal) dan bus berhenti.
- `TestWithDomains_FnErrorPreserved`: `fn` mengembalikan `*errs.Error`; `errors.As` pada hasil `WithDomains` masih menemukannya.
- `TestWithDomains_DBUnreachable`: URL ke port tertutup -> error `open db:`; `fn` tidak dipanggil.

### CLI unit `cmd/cli/main_test.go` (hook `createAdminRun`, tanpa DB)
- `TestCreateAdmin_Success_PrintsLine`: hook sukses; stdout `admin a@b.co dibuat (id <uuid>)\n` (hook palsu mencetak lewat jalur yang sama; atau uji `reportCreateAdminError`/pencetakan sukses sebagai fungsi terpisah), exit 0, stderr kosong.
- `TestCreateAdmin_ConflictMessage`: hook mengembalikan `errs.New(CONFLICT, "email sudah terdaftar")` (dibungkus `fmt.Errorf("x: %w", ...)`); exit 1; stderr persis `email sudah terdaftar\n`; tidak memuat `CONFLICT:`.
- `TestCreateAdmin_ValidationFieldsPrinted`: `errs.WithDetails(INVALID, ..., {"fields": {"password": "minimal 10 karakter", "email": "..."}})` -> stderr memuat `password: minimal 10 karakter`, urut abjad.
- `TestCreateAdmin_MissingTableHint`: `*pgconn.PgError{Code: "42P01"}` -> stderr memuat `grimoire migrate up`.
- `TestCreateAdmin_UnknownErrorGeneric`: error acak bermuatan `"password=rahasia123"` -> stderr `gagal membuat admin; periksa log`, tidak memuat `rahasia123` (kebocoran lewat err).
- `TestCreateAdmin_AlreadyReported_NoDoublePrint`.
- `TestCreateAdmin_PasswordPassedToHook`: hook menerima password dari env; password tidak muncul di stdout/stderr.
- Test validasi flag lama (`TestCreateAdmin_RequiresFlags`, `TestCreateAdmin_UnknownFlag`, test `password_test.go`) tetap lulus tanpa perubahan.

### Integrasi binary `cmd/cli/createadmin_integration_test.go`
Memakai `testSchemaURL(t)` dan `runBin` dari `smoke_integration_test.go`.
- `TestT01_AC1_CreateAdminCLI`:
  1. `migrate up` -> exit 0.
  2. `create-admin --email admin@tim.id --name Admin` dengan `CREATE_ADMIN_PASSWORD=<kuat>` -> exit 0, stdout `admin admin@tim.id dibuat (id <uuid>)`. Query langsung: `role='super_admin'`, `must_change_password=false`, `is_active=true`.
  3. Jalankan lagi dengan `--email ADMIN@tim.id` -> exit 1; stderr memuat `email sudah terdaftar`; stdout dan stderr tidak memuat password; tetap satu baris di `users`.
- `TestCreateAdmin_BeforeMigrate`: schema kosong, tanpa `migrate up` -> exit 1, stderr memuat `grimoire migrate up`.
- `TestCreateAdmin_WeakPassword`: password 5 karakter -> exit 1, stderr `password: minimal 10 karakter`, tidak ada baris.
- `TestCreateAdmin_InvalidConfig`: tanpa `DATABASE_URL`/`SESSION_SECRET` -> exit 1, pesan config, password tidak muncul.
- `TestCreateAdmin_DBUnreachable_NoSecretLeak`: `DATABASE_URL` menunjuk host tertutup dengan password `pw-rahasia` -> exit 1; stdout/stderr tidak memuat `pw-rahasia` maupun password admin.
- `TestCreateAdmin_StdoutClean`: pada sukses, stdout hanya satu baris `admin ... dibuat (id ...)`; log aplikasi hanya di stderr.
- `TestCreateAdmin_PasswordViaStdin`: tanpa env, password dikirim lewat stdin non-TTY -> sukses.
- Hasil login (`POST /api/auth/login`) dengan admin ini diverifikasi di test lintas feature `login` (U4), bukan di sini; di sini cukup `Verify` hash.

### Lintas feature (`auth_integration_test.go`, baris milik usecase ini)
- Tambahkan baris `createadmin` ke tabel `TestT01_AC8_PasswordNeverLoggedOrAudited` (log buffer dan `record.Request` tidak memuat password/hash) dan `TestT01_AC9_EachActionOneAuditEntry` (satu panggilan sukses = satu `user.created`; panggilan CONFLICT = nol).

## E2E Scenarios

Dijalankan terhadap binary yang dibangun (`go build ./cmd/cli`) dan Postgres nyata. Tidak ada UI untuk usecase ini; verifikasi UI lewat login.

Positif:
1. `grimoire migrate up`, lalu `CREATE_ADMIN_PASSWORD='kata-sandi-panjang' grimoire create-admin --email admin@tim.id --name Admin` -> exit 0, `admin admin@tim.id dibuat (id <uuid>)`.
2. Setelah langkah 1, `grimoire serve`; login di `/login` dengan email dan password itu berhasil, langsung ke beranda tanpa diminta ganti password (`must_change_password=false`), menu Users terlihat (bergantung U4/U6 dan frontend).
3. Password lewat pipe: `printf 'kata-sandi-panjang\n' | grimoire create-admin --email a2@tim.id --name Dua` -> exit 0.
4. Email dengan huruf besar/spasi di sekitar nama: `--name "  Budi  "` tersimpan `Budi`; email `Admin2@Tim.ID` tersimpan lowercase.
5. Satu entri audit `user.created` (actor system `cli`) muncul di audit log setelah T02 merge (sebelum itu, log stderr berisi `audit record gagal` dan perintah tetap sukses).

Negatif:
1. Jalankan ulang dengan email sama, termasuk beda huruf besar/kecil -> exit 1, stderr `email sudah terdaftar`, tidak ada baris tambahan.
2. Password 9 karakter -> exit 1, `password: minimal 10 karakter`.
3. Password sama dengan email -> exit 1, `password: password tidak boleh sama dengan email`.
4. Sebelum `migrate up` -> exit 1, `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`.
5. `DATABASE_URL` salah/DB mati -> exit 1 dalam batas 30 detik; password tidak tercetak.
6. Flag tidak lengkap (`--email` saja) -> exit 2 dengan usage (perilaku T00 tetap).
7. Dua terminal menjalankan perintah bersamaan dengan email sama -> satu exit 0, satu exit 1 `email sudah terdaftar`.
8. Nama lebih dari 100 karakter -> exit 1, `name: ...` (pesan dari `ValidateName`).
9. Seluruh keluaran dan log dipindai: password dan hash PHC tidak pernah muncul.

## Request/Response body

Tidak ada endpoint HTTP, sehingga tidak ada JSON di wire. Kontrak berikut adalah tipe Go pada bus (`lib/contracts/core/auth/createadmin`).

### Bus: request dan response sukses

```go
createadmin.Request{Email: "admin@tim.id", Name: "Admin", Password: "<dari CLI; tidak pernah dicatat>"}
createadmin.Response{UserID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d"}
```

### Bus: error (semua `*errs.Error` kecuali infra; dibaca dengan `errors.As`)

Email sudah ada:
```go
&errs.Error{Code: errs.CodeConflict, Message: "email sudah terdaftar"}
```

Validasi gagal (`Details` aman ditampilkan, bentuk sama dengan error HTTP `details`):
```go
&errs.Error{
    Code:    errs.CodeInvalid, // "INVALID_ARGUMENT"
    Message: "permintaan tidak valid",
    Details: map[string]any{"fields": base.FieldErrors{
        "email":    "format email tidak valid",
        "password": "minimal 10 karakter",
    }},
}
```
Pesan field yang mungkin: `email: format email tidak valid`; `name: ...` (kosong, lebih dari 100 karakter, karakter kontrol); `password: minimal 10 karakter`, `password: maksimal 256 byte`, `password: password tidak boleh sama dengan email`.

Kegagalan infra: error terbungkus biasa, mis. `createadmin: insert user: <penyebab>`; bukan `*errs.Error`. Tidak pernah memuat password.

Tipe request salah: `bus.Call(ctx, "core.auth.create-admin", "string-salah")` -> error membungkus `bus.ErrBadRequestType`.

### Record audit yang dikirim (`core.audit.record`)

```go
record.Request{
    ActorType: "system", ActorID: "", ActorLabel: "cli",
    Action: "user.created", TargetType: "user", Target: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    Args:   map[string]any{"email": "admin@tim.id", "name": "Admin", "role": "super_admin", "generated_password": false},
    Status: "ok", IP: "", UserAgent: "", TraceID: "",
}
```

### CLI

Sukses (exit 0, stdout):
```
$ CREATE_ADMIN_PASSWORD='kata-sandi-panjang' grimoire create-admin --email admin@tim.id --name Admin
admin admin@tim.id dibuat (id 0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d)
$ echo $?
0
```

Email sudah ada (exit 1, stderr):
```
$ CREATE_ADMIN_PASSWORD='kata-sandi-panjang' grimoire create-admin --email ADMIN@tim.id --name Admin
email sudah terdaftar
$ echo $?
1
```

Validasi (exit 1, stderr):
```
$ CREATE_ADMIN_PASSWORD='pendek' grimoire create-admin --email admin@tim.id --name Admin
permintaan tidak valid
  password: minimal 10 karakter
$ echo $?
1
```

Migrasi belum dijalankan (exit 1, stderr):
```
tabel belum ada; jalankan "grimoire migrate up" lebih dulu
```

Config tidak valid (exit 1, stderr; dicetak oleh `server.LoadConfig`, tanpa nilai secret):
```
<pesan validasi config, mis. menyebut DATABASE_URL dan SESSION_SECRET>
```

Kegagalan lain (exit 1, stderr; detail hanya di log stderr, tanpa password):
```
gagal membuat admin; periksa log
```

Flag/argumen salah: exit 2 dengan usage (perilaku T00, tidak berubah).
