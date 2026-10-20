# Usecase Plan: cli-and-wiring (T00 Foundation, langkah 7)

Overview induk: `docs/superpowers/plans/foundation/overview.md`. Tiket: `docs/tickets/T00-foundation.md`. Aturan: `rules.txt` (4.6, 4.7, 8.3), `docs/tickets/README.md`.
Konteks yang dicek: belum ada `CLAUDE.md`, belum ada `.claude/rules/`, belum ada kode di `backend/`. Plan ini mengasumsikan usecase 1-6 sudah selesai dan memakai API publik yang didefinisikan overview (bagian 4, 5, 8, 9).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | cli-and-wiring (satu PR koheren) |
| Scope | fullstack (Go entrypoint + makefile yang membangun FE; tidak ada kode FE baru) |
| Domain | infra |
| Module | foundation |
| Resource root | `backend/internal/infra` (kode usecase ini berada di `backend/cmd/**` dan `makefile`; `infra/app.Deps` difinalkan di resource root) |
| AC yang dicakup | T00-AC2 (binary), T00-AC1 (target `make dev`) |
| Bergantung pada | usecase 2 (bus), 3 (db-migrations), 4 (jobs), 5 (http-server), 6 (static) |

| Endpoint | Method | Version | Keterangan |
|---|---|---|---|
| (tidak ada endpoint baru) | - | - | Usecase ini merakit `/healthz`, `/readyz`, `/api/**`, `/mcp/**`, dan static SPA yang dimiliki usecase 5 dan 6. Tidak ada route baru |

Subcommand CLI (antarmuka pengguna usecase ini):

| Subcommand | Efek | Butuh env/config |
|---|---|---|
| `grimoire` / `grimoire serve` | jalankan server | ya |
| `grimoire migrate up` | jalankan migrasi lalu keluar | ya |
| `grimoire create-admin --email E --name N` | validasi, lalu keluar 1 sampai T01 tersedia | tidak (T00 tidak membuka DB) |
| `grimoire version` | cetak versi | tidak |
| `grimoire help`, `-h`, `--help` | cetak usage | tidak |

Endpoint bus: provider tidak ada. Consumer: tidak ada pada T00 (`core.auth.create-admin` ditambahkan T01, lihat bagian 3 langkah 6). Aksi audit: tidak ada.

Pola `init/handler/params/usecase/repository` TIDAK berlaku. Rule 4.7 berlaku: `cmd/**` hanya merakit. Rule 4.6: perakitan yang meng-import `internal/domain/*` berada di `cmd/internal/server`, bukan di `infra/`.

## 2. Summary Plan

Menyatukan semua paket infra menjadi satu binary `bin/grimoire`.

- `cmd/internal/server` (package `server`): `Run(ctx, cfg, log, opts...) error` berisi urutan perakitan dan graceful shutdown; `MigrateUp(...)` untuk `migrate up`; `ServeFromEnv(...)` pembungkus (load config, logger, signal, Run, exit code) yang dipakai bersama oleh `cmd/http` dan `cmd/cli` agar kedua `main` tetap tipis.
- `cmd/cli` (package `main`): parser subcommand/flag, resolusi password, dispatch. Seluruh logika dibungkus fungsi `run(...) int` yang menerima stdin/stdout/stderr/getenv/isTTY sehingga bisa dites tanpa TTY dan tanpa `os.Exit`.
- `cmd/http`: `main` murni memanggil `server.ServeFromEnv`.
- `cmd/internal/buildinfo`: variabel ldflags `Version`, `Commit`, `BuildDate`.
- `makefile` dilengkapi: `dev`, `build`, `test`, `test-integration`, `test-e2e`, `lint`, `migrate`, `api-bundle`, `api-check`.
- `infra/app.Deps` difinalkan (field `Ctx`, `DB`, `Bus`, `Jobs`, `API`, `MCP`).

Keputusan penting:
1. Signature `Run` tetap tiga argumen wajib sesuai overview, ditambah opsi fungsional variadic (`...Option`) murni untuk test: `WithListenReady(chan<- string)` (melaporkan alamat nyata saat `HTTP_ADDR=127.0.0.1:0`) dan `WithAppHook(func(app.Deps))` (test mendaftarkan route lambat). Pemanggil produksi tidak memakai opsi.
2. Penanganan sinyal ada di `ServeFromEnv` (`signal.NotifyContext` SIGINT/SIGTERM), bukan di `Run`. `Run` kembali setelah `ctx` dibatalkan dan shutdown selesai. Itu membuat `Run` bisa dites dengan membatalkan context. Sinyal kedua setelah yang pertama memakai perilaku default (proses langsung mati) lewat memanggil `stop()` segera setelah ctx batal.
3. Listener dibuat dengan `net.Listen` lebih dulu, lalu `srv.Serve(ln)`. Gagal bind terdeteksi sebelum jobs dijalankan dan alamat `:0` bisa diketahui.
4. `create-admin` di T00 hanya memvalidasi lalu gagal dengan pesan jelas tanpa membuka DB (keputusan overview 8.10).
5. Tidak ada env var baru. `CREATE_ADMIN_PASSWORD` sudah didokumentasikan di `docs/configuration.md` bagian env CLI (usecase 1); langkah 8 memverifikasinya.

## 3. Plan (langkah berurutan)

### Langkah 1. buildinfo
`backend/cmd/internal/buildinfo/buildinfo.go`:
```go
package buildinfo

// Diisi lewat -ldflags "-X grimoire/cmd/internal/buildinfo.Version=..." dst.
var (
    Version   = "dev"
    Commit    = "none"
    BuildDate = "unknown"
)

// String: "grimoire <Version> (commit <Commit>, built <BuildDate>)"
func String() string
```
Hapus `.keep` di folder itu.

### Langkah 2. Finalisasi `infra/app.Deps`
Edit `backend/internal/infra/app/deps.go` menjadi struct lengkap overview 4.4 (`Ctx`, `Config`, `Log`, `DB *db.DB`, `Bus *bus.Registry`, `Jobs *jobs.Scheduler`, `API chi.Router`, `MCP chi.Router`). Hanya menambah field; tidak ada logic. Jangan mengubah `Config` dan `Log` yang sudah ada.

### Langkah 3. `cmd/internal/server/run.go`
Hapus `.keep`. Isi:

```go
package server

type Option func(*runOpts)
type runOpts struct {
    listenReady chan<- string // alamat nyata listener
    appHook     func(app.Deps) // dipanggil setelah Register domain, sebelum static dipasang
}
func WithListenReady(ch chan<- string) Option
func WithAppHook(fn func(app.Deps)) Option

func Run(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...Option) error
func MigrateUp(ctx context.Context, cfg config.Config, log *slog.Logger, out io.Writer) error
func ServeFromEnv(ctx context.Context, getenv func(string) string, stderr io.Writer) int
```

Urutan `Run` (tiap langkah yang gagal mengembalikan error yang membungkus nama langkah, setelah membersihkan yang sudah dibuka lewat `defer` LIFO yang disusun sesuai bagian shutdown):
1. `log.Info("starting", "version", buildinfo.String(), "config", cfg)` (LogValuer menyamarkan secret).
2. `d, err := db.Open(ctx, cfg)`; `defer` ditunda ke fase shutdown eksplisit (lihat 9), tetapi jika langkah setelahnya gagal sebelum server jalan, tutup DB.
3. Bila `cfg.RunMigrationsOnStart`: `db.Migrate(ctx, d, migrations.FS, log)` (`migrations` = `grimoire/migrations`); log jumlah file diterapkan. Gagal = `Run` gagal (server tidak start).
4. `b := bus.NewRegistry(log)`; `sched := jobs.New(d, log)`.
5. `srvCtx, cancelSrv := context.WithCancel(context.WithoutCancel(ctx))`. Ctx server ini TIDAK ikut batal saat sinyal; dibatalkan manual SETELAH HTTP drain agar janitor rate limiter dan goroutine module tetap hidup selama request berjalan.
6. `root, api, mcp := httpx.NewRouter(cfg, log, b, d)` (`*db.DB` memenuhi `Pinger`).
7. `deps := app.Deps{Ctx: srvCtx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: sched, API: api, MCP: mcp}`.
8. `core.Register(deps)`, `project.Register(deps)`, `discovery.Register(deps)`. Urutan tidak boleh berpengaruh; test memverifikasi (bagian 6). Import: `grimoire/internal/domain/core`, `.../project`, `.../discovery`.
9. Jalankan `appHook(deps)` bila ada (hanya test).
10. `h, err := static.New(cfg, static.Embedded(), log)`; `root.NotFound(h.ServeHTTP)`. Method selain GET/HEAD pada path non-API ditangani handler static (405). Sebelum memasang, pastikan `httpx.NewRouter` sudah me-mount `api` di `/api` dan `mcp` di `/mcp` dan `/api/**` yang tidak cocok dijawab 404 JSON oleh sub-router `api` (bukan jatuh ke static). Test T00-AC2 mencakup ini.
11. `sched.Start(srvCtx)`.
12. `ln, err := net.Listen("tcp", cfg.HTTPAddr)`; kirim `ln.Addr().String()` ke `listenReady` bila ada (non-blocking send pada channel ber-buffer 1; opsi test wajib membuat buffer).
13. `srv := httpx.NewServer(cfg, root)` (`ReadHeaderTimeout`, `IdleTimeout=120s`; BaseContext = `srvCtx` agar handler melihat ctx server). `go srv.Serve(ln)` mengirim hasil ke `errCh`; log `"listening" addr=...`.
14. `select { case <-ctx.Done(): ; case err := <-errCh: (bukan http.ErrServerClosed) -> catat sebagai failure dan lanjut ke shutdown }`.
15. Shutdown berurutan (bagian 9), kumpulkan error dengan `errors.Join`, jangan berhenti di tengah.

`MigrateUp`: load sudah dilakukan pemanggil. `db.Open`, `db.Migrate(ctx, d, migrations.FS, log)`, `d.Close()`. Cetak ke `out`: satu baris `diterapkan: <nama file>` per migrasi, atau `tidak ada migrasi baru` bila `Applied` kosong. Error dikembalikan.

`ServeFromEnv(ctx, getenv, stderr) int`:
1. `cfg, err := config.Load(getenv)`; error -> tulis `err.Error()` (pesan multi-baris `konfigurasi tidak valid:...`) ke `stderr`, return 1. Logger belum ada, jadi tulis langsung, dan jangan pernah mencetak nilai secret.
2. `log := logger.New(cfg)`.
3. `ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`; goroutine `<-ctx.Done(); stop()` agar sinyal kedua default-kill.
4. `Run(ctx, cfg, log)`; error -> `log.Error("server berhenti dengan error", "error", err)`, return 1; sukses return 0.

Fungsi `migrate` memakai helper bersama `loadConfig(getenv, stderr) (config.Config, *slog.Logger, bool)` supaya validasi/pesan identik dengan serve.

### Langkah 4. `cmd/http/main.go`
Hapus `.keep`. Seluruh isi:
```go
func main() {
    os.Exit(server.ServeFromEnv(context.Background(), os.Getenv, os.Stderr))
}
```
Dipakai `go run ./cmd/http` oleh `make dev`.

### Langkah 5. `cmd/cli`
Hapus `.keep`. File:

**`cmd/cli/main.go`**
```go
func main() {
    c := newCLI(os.Stdin, os.Stdout, os.Stderr, os.Getenv)
    os.Exit(c.run(context.Background(), os.Args[1:]))
}
```

**`cmd/cli/cli.go`**
```go
type cli struct {
    stdin          io.Reader
    stdout, stderr io.Writer
    getenv         func(string) string
    isTTY          func() bool                   // default: term.IsTerminal(int(os.Stdin.Fd()))
    readSecret     func(prompt string) (string, error) // default: term.ReadPassword pada stdin fd, tulis prompt ke stderr
    serve          func(ctx context.Context) int       // default: server.ServeFromEnv(ctx, getenv, stderr)
    migrateUp      func(ctx context.Context) int       // default: loadConfig + server.MigrateUp
}
func newCLI(stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *cli
func (c *cli) run(ctx context.Context, args []string) int
```
Aturan parsing `run`:
- `len(args)==0` -> `serve`.
- `args[0]` salah satu `help`, `-h`, `--help` -> usage ke stdout, return 0.
- `serve`: tidak menerima argumen lain (ada -> usage ke stderr, return 2).
- `migrate`: `args[1]` harus `up` dan tanpa argumen lain; selain itu return 2 dengan usage.
- `version`: cetak `buildinfo.String()` + `\n` ke stdout, return 0; tidak memuat config.
- `create-admin`: lihat langkah 6.
- argumen pertama diawali `-` selain help (mis. `--foo`) atau subcommand tak dikenal: `perintah tidak dikenal: "<x>"` + usage ke stderr, return 2.
- Flag memakai `flag.NewFlagSet(name, flag.ContinueOnError)` dengan output `io.Discard` dan pesan error sendiri agar format konsisten; `flag.ErrHelp` -> usage return 0.

Usage (satu sumber `const usage`):
```
Pemakaian: grimoire [perintah]

Perintah:
  serve                                    jalankan server (default)
  migrate up                               jalankan migrasi database
  create-admin --email E --name N          buat akun super admin pertama
  version                                  tampilkan versi
  help                                     tampilkan bantuan

Env wajib untuk serve/migrate: DATABASE_URL, SESSION_SECRET. Lihat docs/configuration.md.
```

**`cmd/cli/password.go`**
```go
func (c *cli) resolvePassword() (string, error)
```
Urutan:
1. `CREATE_ADMIN_PASSWORD` non-kosong (tanpa trim di tengah; `strings.TrimRight(v, "\r\n")`) -> pakai.
2. Bila `!c.isTTY()`: baca satu baris dari `c.stdin` (`bufio.Reader.ReadString('\n')`, EOF tanpa newline diterima), buang `\r\n`. Tidak ada input -> error.
3. Bila TTY: `readSecret("Password: ")` dan `readSecret("Ulangi password: ")`; beda -> error `password tidak sama`.
4. Hasil kosong -> error `password tidak boleh kosong`.
Password tidak pernah dicetak, di-log, atau muncul di pesan error.

### Langkah 6. `create-admin`
`runCreateAdmin` terbagi dua supaya T01 hanya mengganti satu fungsi:
```go
type createAdminArgs struct{ Email, Name, Password string }

func (c *cli) createAdmin(ctx context.Context, args []string) int {
    // 1. parse --email, --name (FlagSet). Argumen posisional ekstra -> return 2.
    // 2. validasi: Name = strings.TrimSpace non-kosong; Email lolos mail.ParseAddress dan addr.Address == input (tanpa display name).
    //    Gagal -> pesan "--email wajib dan harus alamat email valid" / "--name wajib diisi" + usage, return 2.
    // 3. password, err := c.resolvePassword(); err -> pesan, return 2.
    // 4. if err := runCreateAdmin(ctx, createAdminArgs{...}); err != nil { fmt.Fprintln(c.stderr, err); return 1 }
    //    return 0
}

// TODO(T01): ganti isi fungsi ini. Rakit config, db.Open, bus, core.Register (lewat
// server helper), lalu bus.Call(constants.BUS_CORE_AUTH_CREATE_ADMIN, ...) dan
// cetak "admin <email> dibuat". Tambahkan konstanta + kontrak + baris katalog bus
// di tiket T01 (T00 hanya mencatat TODO di docs/tickets/README.md).
func runCreateAdmin(ctx context.Context, a createAdminArgs) error {
    return errors.New("create-admin belum tersedia (butuh T01)")
}
```
Catatan: urutan validasi flag -> password -> baru error T01, sehingga pengguna mendapat umpan balik argumen yang benar sebelum pesan ketersediaan. Pesan error T01 persis `create-admin belum tersedia (butuh T01)` ke stderr, exit 1, tanpa membuka DB.

### Langkah 7. Makefile
Edit `makefile` (kerangka dari usecase 1). Indentasi recipe memakai TAB. Isi final:

```make
SHELL := /bin/bash
-include .env
export

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := grimoire/cmd/internal/buildinfo
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(DATE)
COMPOSE := docker compose -f docker-compose.dev.yml
DIST    := backend/internal/infra/static/dist
REDOCLY := @redocly/cli@<versi-pin-dari-overview>   # pin satu versi, samakan dengan frontend/package.json bila ada
TEST_DATABASE_URL ?= postgres://grimoire:grimoire@localhost:5432/grimoire?sslmode=disable

.PHONY: dev dev-db dev-backend dev-frontend build test test-integration test-e2e lint migrate api-bundle api-check

dev: dev-db
	$(MAKE) -j2 dev-backend dev-frontend

dev-db:
	$(COMPOSE) up -d --wait db

dev-backend:
	cd backend && APP_ENV=development RUN_MIGRATIONS_ON_START=true COOKIE_SECURE=false \
	  DEV_VITE_URL=http://localhost:5173 go run ./cmd/http

dev-frontend:
	cd frontend && npm run dev

build:
	cd frontend && npm ci && npm run build
	rm -rf $(DIST)/* && cp -R frontend/dist/. $(DIST)/
	mkdir -p bin
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/grimoire ./cmd/cli
	-git checkout -- $(DIST)/index.html 2>/dev/null   # kembalikan placeholder yang di-commit

test:
	cd backend && go test -race ./...
	cd frontend && npm test -- --run

test-integration: dev-db
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -tags integration ./...

test-e2e: build
	scripts/e2e.sh   # LIHAT catatan: jalankan bin/grimoire di background, npx playwright test, matikan proses

lint:
	cd backend && go vet ./... && golangci-lint run
	cd frontend && npm run check && npx depcruise src

migrate:
	cd backend && go run ./cmd/cli migrate up

api-bundle:
	npx --yes $(REDOCLY) bundle api/root.yml -o api/openapi.yml

api-check:
	@tmp=$$(mktemp) && npx --yes $(REDOCLY) bundle api/root.yml -o $$tmp \
	  && { diff -u api/openapi.yml $$tmp || { echo "api/openapi.yml usang: jalankan make api-bundle"; rm -f $$tmp; exit 1; }; } ; rm -f $$tmp
```
Catatan implementasi:
- `test-e2e` tidak boleh memakai `scripts/` baru bila tidak diinginkan; alternatif tanpa file baru: satu recipe shell berbaris `\` yang menjalankan `bin/grimoire &`, menyimpan PID, `trap 'kill $$pid' EXIT`, menunggu `curl -sf localhost:8080/healthz`, lalu `cd frontend && npx playwright test`. Pilih recipe inline agar tidak menambah file di luar daftar Changes overview.
- `export` dari `.env` membuat `DATABASE_URL`/`SESSION_SECRET` dari `.env` terbaca `go run`. `make dev` gagal cepat dengan pesan config bila `.env` belum ada (perilaku yang diinginkan; `docs/configuration.md` menjelaskan `cp .env.example .env`).
- `api-bundle` dan `api-check` bergantung pada `api/root.yml` yang dibuat usecase 8 (`architecture-guard`). Sampai usecase 8 selesai target ini gagal dengan pesan redocly "file tidak ada"; ini diterima karena usecase 8 berada setelah ini dalam urutan, dan CI job `api-check` baru ditambahkan di usecase 8.
- `build` memakai `npm ci` sehingga `frontend/package-lock.json` wajib di-commit (usecase 6).

### Langkah 8. Dokumentasi dan verifikasi dokumen
1. `docs/configuration.md`: pastikan bagian "Env di luar Config" memuat `CREATE_ADMIN_PASSWORD` dan penjelasan sumber password (env, stdin non-TTY, prompt TTY). Tambahkan bagian "Menjalankan" (`grimoire serve`, `migrate up`, `create-admin`, `version`, exit code 0/1/2) dan "Shutdown" (urutan dan `SHUTDOWN_TIMEOUT`, `BUS_SHUTDOWN_TIMEOUT`). Tidak menambah env.
2. `docs/tickets/README.md`: bila belum ada baris TODO `core.auth.create-admin` (usecase 2 / overview 5.4), tambahkan. Jangan ubah selain itu.

### Langkah 9. Shutdown (rinci urutan di `Run`)
Setelah `ctx` batal atau Serve error:
1. `shCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)`; `srv.Shutdown(shCtx)` (berhenti menerima koneksi, menunggu request berjalan). Error (termasuk deadline) dicatat; bila deadline lewat, `srv.Close()` untuk memutus sisa koneksi.
2. `cancelSrv()` (ctx server batal: janitor rate limiter dan goroutine module berhenti).
3. `sched.Stop(stopCtx)` dengan timeout `cfg.ShutdownTimeout`.
4. `b.Shutdown(busCtx)` dengan timeout `cfg.BusShutdownTimeout` (menunggu goroutine `Publish`). Urutan ini setelah HTTP drain, sehingga Publish yang dipicu request terakhir ikut tertunggu.
5. `d.Close()`.
Semua error dikumpulkan dengan `errors.Join`; `Run` mengembalikan `nil` hanya bila semuanya sukses. Log tiap tahap (`"shutdown: http selesai"`, dst.).

### Langkah 10. Verifikasi akhir
`cd backend && go mod tidy` (menambah `golang.org/x/term` bila belum), `go build ./...`, `go vet ./...`, `go test -race ./...`, `make test-integration`, `make build && ./bin/grimoire version`. Jalankan archtest (`go test ./internal/archtest`): `cmd/**` boleh meng-import domain dan infra; `infra/**` tetap tidak meng-import domain.

## 4. Migration

Tidak ada file migrasi. Usecase ini hanya memanggil runner `db.Migrate` dengan `migrations.FS` (folder `migrations/core` dan `migrations/project` masih kosong sampai T01/T03).

## 5. Changes (per file/folder)

Baru:
- `backend/cmd/internal/buildinfo/buildinfo.go`
- `backend/cmd/internal/server/run.go`
- `backend/cmd/internal/server/run_integration_test.go` (build tag `integration`)
- `backend/cmd/internal/server/run_test.go` (unit tanpa DB, bila ada fungsi murni, mis. `loadConfig` dengan getenv palsu)
- `backend/cmd/http/main.go`
- `backend/cmd/cli/main.go`, `cli.go`, `password.go`
- `backend/cmd/cli/main_test.go`, `password_test.go`
- `backend/cmd/cli/smoke_integration_test.go` (build tag `integration`; membangun dan menjalankan binary)

Diubah:
- `backend/internal/infra/app/deps.go` (finalisasi field)
- `makefile` (target final)
- `docs/configuration.md` (bagian Menjalankan, Shutdown; verifikasi `CREATE_ADMIN_PASSWORD`)
- `docs/tickets/README.md` (hanya bila baris TODO create-admin belum ada)
- `backend/go.mod`, `backend/go.sum` (`golang.org/x/term`)

Dihapus: `.keep` di `backend/cmd/http`, `backend/cmd/cli`, `backend/cmd/internal/server`, `backend/cmd/internal/buildinfo`.

Tidak disentuh: semua paket infra lain, domain stub (kecuali dipanggil), frontend, `rules.txt`, plan usecase lain.

## 6. Test Scenario

Konvensi rule 13.4 (`TestT00_ACn_...` untuk AC). Test integrasi: build tag `integration`, skip dengan pesan `TEST_DATABASE_URL tidak di-set; lewati test integrasi` secara lokal dan gagal bila `CI=true`. Jalankan dengan `-race`.

### cmd/cli (`main_test.go`, unit, tanpa DB dan tanpa TTY)
Helper `newTestCLI(t, env map[string]string, stdin string, tty bool)` membuat `cli` dengan buffer stdout/stderr, `isTTY` palsu, `serve` dan `migrateUp` stub yang mencatat pemanggilan.
- `TestCLI_DefaultIsServe`: `run([])` memanggil stub serve sekali, tidak memanggil yang lain.
- `TestCLI_ServeExplicit`: `run(["serve"])` sama; `run(["serve","x"])` -> 2.
- `TestCLI_MigrateUp`: `["migrate","up"]` memanggil stub migrate; `["migrate"]`, `["migrate","down"]`, `["migrate","up","x"]` -> 2 dan stub tidak terpanggil.
- `TestCLI_UnknownSubcommandExit2`: `["frobnicate"]`, `["--foo"]` -> 2, stderr memuat `perintah tidak dikenal` dan `Pemakaian:`; stdout kosong.
- `TestCLI_HelpExit0`: `help`, `-h`, `--help` -> 0, usage di stdout.
- `TestCLI_Version`: set `buildinfo.Version/Commit/BuildDate` di test (simpan dan pulihkan lewat `t.Cleanup`), output persis `grimoire v1.2.3 (commit abc1234, built 2026-10-20T00:00:00Z)\n`, exit 0, tanpa memanggil `getenv` untuk config (getenv palsu panic bila dipanggil).
- `TestCLI_VersionDefaults`: tanpa set -> `grimoire dev (commit none, built unknown)`.
- `TestCreateAdmin_RequiresFlags`: tanpa flag, hanya `--email`, hanya `--name`, nama spasi saja, email tanpa `@`, email `Nama <a@b.c>` -> semua 2 dengan pesan spesifik; `runCreateAdmin` tidak tercapai (verifikasi lewat password tidak dibaca: stdin tidak dikonsumsi).
- `TestCreateAdmin_UnknownFlag`: `--bogus` -> 2.
- `TestCreateAdmin_NotAvailableUntilT01`: argumen valid + `CREATE_ADMIN_PASSWORD=rahasia123` -> exit 1; stderr persis memuat `create-admin belum tersedia (butuh T01)`; stdout dan stderr TIDAK memuat `rahasia123`; stub serve/migrate tidak dipanggil.

### cmd/cli (`password_test.go`)
- `TestPassword_FromEnvWins`: env terisi dan stdin berisi nilai lain -> env dipakai, stdin tidak dibaca.
- `TestPassword_FromStdinNonTTY`: `stdin="pw-dari-stdin\n"` -> `pw-dari-stdin`; varian `\r\n`; varian tanpa newline akhir.
- `TestPassword_StdinOnlyFirstLine`: `"a\nb\n"` -> `a`.
- `TestPassword_EmptyRejected`: stdin kosong, stdin `"\n"`, env berisi hanya spasi? (env `""` dianggap tidak di-set dan jatuh ke stdin; stdin kosong -> error).
- `TestPassword_TTYPromptsTwice`: `isTTY=true`, `readSecret` palsu mengembalikan `x`,`x` -> `x`; kedua panggilan memakai prompt berbeda; stdin tidak dibaca.
- `TestPassword_TTYMismatch`: `x`,`y` -> error `password tidak sama`, exit 2 melalui `createAdmin`.
- `TestPassword_TTYReadError`: `readSecret` error -> exit 2.
- `TestPassword_NeverEchoed`: jalankan semua jalur dengan password penanda `S3CR3T-MARKER`; assert stdout+stderr tidak memuat penanda.

### cmd/internal/server (`run_test.go`, unit)
- `TestLoadConfig_PrintsAllProblemsAndFails`: getenv kosong -> return false, stderr memuat `konfigurasi tidak valid:`, `DATABASE_URL`, `SESSION_SECRET`; tidak memuat nilai secret yang diberikan pada kasus lain (secret pendek `abc`).
- `TestServeFromEnv_InvalidConfigReturns1`: exit 1 tanpa membuka DB (tidak ada listener).

### cmd/internal/server (`run_integration_test.go`, DB nyata)
Helper `testEnv(t)`: membuat schema unik lewat koneksi biasa ke `TEST_DATABASE_URL`, menambahkan parameter `search_path=<schema>` ke URL untuk `cfg.DatabaseURL`, drop schema saat cleanup; `cfg` dibangun via `config.Load` dengan hanya `DATABASE_URL`, `SESSION_SECRET` (32+ byte), `HTTP_ADDR=127.0.0.1:0`, `COOKIE_SECURE=false`. `startRun(t, cfg, opts...)` menjalankan `Run` di goroutine, menunggu alamat dari `WithListenReady`, mengembalikan `baseURL`, `cancel`, `done <-chan error`.
- `TestT00_AC2_ServeWithOnlyRequiredEnv`: `GET /` 200 `text/html`, `Cache-Control: no-cache`; `GET /projects/abc` 200 HTML (fallback SPA); `GET /healthz` 200 `{"status":"ok"}`; `GET /readyz` 200; `GET /assets/missing.js` 404; `GET /api/nope` 404 JSON `NOT_FOUND` dengan `trace_id`; `DELETE /healthz` 405; `POST /api/x` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`. Setelah `cancel()`, `done` menghasilkan `nil` dalam 5 detik.
- `TestServe_GracefulShutdownWaitsInflight`: `WithAppHook` mendaftarkan `GET /api/_test/slow` yang menutup channel `started` lalu tidur 500ms dan menjawab `200 slow-ok`. Kirim request di goroutine, tunggu `started`, panggil `cancel()`. Assert: respons 200 `slow-ok` diterima utuh; `Run` kembali `nil` SETELAH respons selesai (catat waktu); request baru setelah listener tutup gagal koneksi.
- `TestServe_ShutdownTimeoutReturnsError`: `SHUTDOWN_TIMEOUT=100ms`, handler tidur 2s -> `Run` mengembalikan error yang `errors.Is(err, context.DeadlineExceeded)`; waktu total jauh di bawah 2s.
- `TestServe_BusPublishDrainedOnShutdown`: `WithAppHook` mendaftarkan endpoint bus dan route yang `Publish`; handler Publish tidur 200ms lalu set flag; `cancel()` tepat setelah respons; `Run` kembali dan flag sudah ter-set (bus menunggu).
- `TestServe_RunMigrationsOnStart`: `RUN_MIGRATIONS_ON_START=true` -> tabel `schema_migrations` ada di schema uji setelah start; tanpa flag -> tabel tidak ada.
- `TestServe_RegisterOrderIndependent`: dua `Run` berurutan dengan hook yang mencatat bahwa `Register` domain dipanggil (stub no-op) tidak mengubah hasil; fokus pada `httpx` hanya bila stub domain tidak bisa diamati — bila tidak bermakna, tandai test ini sebagai tanggung jawab bus (AC3) dan hapus dari usecase ini.
- `TestServe_BindFailureReturnsError`: `HTTP_ADDR` pada port yang sudah dipakai listener lain -> `Run` error, DB tertutup (tidak ada goroutine bocor; cek dengan `goleak`-style hitung sederhana bila tersedia, bila tidak lewati).
- `TestServe_DBDownFailsFast`: `DATABASE_URL` menunjuk port mati -> `Run` error dalam batas ping 5 detik, server tidak listen.
- `TestMigrateUp_PrintsApplied`: FS `migrations` produksi kosong -> output `tidak ada migrasi baru`; dijalankan dua kali tetap sama (T00-AC5 sisi CLI).

### cmd/cli (`smoke_integration_test.go`, binary nyata, T00-AC2)
- `TestT00_AC2_BuiltBinaryServesWithOnlyRequiredEnv`: `go build -ldflags "-X ...Version=test-ver ..." -o <tmp>/grimoire ./cmd/cli` dari direktori `backend/` (path dicari dengan `runtime.Caller`), `exec.Command` dengan `Dir = t.TempDir()` (direktori kosong, bukti tak butuh file lain), `Env` HANYA `DATABASE_URL` (schema uji seperti di atas), `SESSION_SECRET`, dan `HTTP_ADDR=127.0.0.1:<port-bebas>` (satu-satunya tambahan, supaya tidak bentrok port). Tunggu `/healthz` 200 (polling maks 10s). Assert `GET /` 200 HTML, `GET /healthz` 200. Kirim `SIGTERM`; proses keluar dengan status 0 dalam 10 detik.
- `TestBuiltBinary_VersionAndHelp`: `grimoire version` dengan env kosong total -> `grimoire test-ver (commit ..., built ...)`, exit 0 (membuktikan `version` tidak butuh env).
- `TestBuiltBinary_MissingEnvExit1`: `grimoire` tanpa env -> exit 1, stderr memuat `DATABASE_URL` dan `SESSION_SECRET` sekaligus.
- `TestBuiltBinary_CreateAdminNotAvailable`: `create-admin --email a@b.co --name Admin` dengan `CREATE_ADMIN_PASSWORD` -> exit 1, pesan `create-admin belum tersedia (butuh T01)`.

### Makefile (verifikasi di CI, bukan Go test)
- `make -n build` memuat `-X grimoire/cmd/internal/buildinfo.Version=`.
- `make build` lalu `git status --porcelain backend/internal/infra/static/dist` bersih (placeholder dipulihkan).
- `make api-check` setelah usecase 8: exit 0 pada repo bersih.

## 7. E2E Scenarios

Positif:
1. **T00-AC1** `make dev`: Postgres sehat (`--wait`), `go run ./cmd/http` di `:8080` dengan migrasi otomatis, Vite di `:5173`. Buka `http://localhost:8080/` -> halaman kosong berjudul i18n; ubah file Svelte -> HMR tampil lewat proxy; Ctrl-C menghentikan keduanya.
2. **T00-AC2** `make build` -> `bin/grimoire` disalin ke direktori kosong, `DATABASE_URL` + `SESSION_SECRET` -> `GET /` 200 HTML, `GET /projects/abc` 200 HTML, `GET /assets/<hash>.js` 200 `immutable`, `GET /healthz` 200.
3. `bin/grimoire` tanpa argumen sama dengan `bin/grimoire serve`.
4. `bin/grimoire migrate up` pada DB kosong -> exit 0, `tidak ada migrasi baru` (belum ada file), tabel `schema_migrations` ada; ulang -> exit 0 sama.
5. `bin/grimoire version` -> `grimoire v0.1.0 (commit 1a2b3c4, built 2026-10-20T08:00:00Z)`.
6. SIGTERM saat request lambat sedang berjalan -> request selesai 200, proses keluar 0 dalam `SHUTDOWN_TIMEOUT`.
7. `bin/grimoire help` -> usage, exit 0.
8. Playwright smoke (`make test-e2e`) terhadap binary hasil build: buka `/`, tanpa error console, `/healthz` 200.

Negatif:
1. Start tanpa `DATABASE_URL`, `SESSION_SECRET` pendek -> exit 1, kedua error tercetak sekaligus, tanpa nilai secret.
2. Start dengan DB mati -> exit 1 dalam kurang dari ~6 detik, pesan koneksi DB tanpa password pada URL.
3. `RUN_MIGRATIONS_ON_START=true` dengan migrasi rusak -> exit 1, server tidak listen.
4. `HTTP_ADDR` sudah dipakai -> exit 1 dengan pesan bind.
5. `create-admin` tanpa flag -> exit 2 dengan usage; `--email bukan-email --name X` -> exit 2.
6. `create-admin --email a@b.co --name Admin` + password valid -> exit 1 `create-admin belum tersedia (butuh T01)`; password tidak tercetak.
7. `printf '' | bin/grimoire create-admin --email a@b.co --name Admin` (stdin kosong, bukan TTY, tanpa env) -> exit 2 `password tidak boleh kosong`.
8. `bin/grimoire frobnicate` -> exit 2 dengan usage; `bin/grimoire migrate` -> exit 2.
9. Sinyal kedua (Ctrl-C dua kali) saat shutdown lambat -> proses langsung mati.
10. SIGTERM saat request melebihi `SHUTDOWN_TIMEOUT` -> koneksi diputus, exit 1 dengan log `shutdown: http melewati batas waktu`.

## 8. Request/Response body

### CLI

`bin/grimoire version` (stdout, exit 0):
```
grimoire v0.1.0 (commit 1a2b3c4, built 2026-10-20T08:00:00Z)
```

`bin/grimoire migrate up` (stdout, exit 0):
```
diterapkan: 20261020101500000_create_users.sql
diterapkan: 20261020101600000_create_sessions.sql
```
atau
```
tidak ada migrasi baru
```

`bin/grimoire frobnicate` (stderr, exit 2):
```
perintah tidak dikenal: "frobnicate"

Pemakaian: grimoire [perintah]
...
```

`bin/grimoire create-admin` (stderr, exit 2):
```
--email wajib dan harus alamat email valid

Pemakaian: grimoire [perintah]
...
```

`bin/grimoire create-admin --email a@b.co --name Admin` dengan password valid (stderr, exit 1):
```
create-admin belum tersedia (butuh T01)
```

Config tidak valid (stderr, exit 1):
```
konfigurasi tidak valid:
- DATABASE_URL: wajib diisi
- SESSION_SECRET: minimal 32 byte (sekarang 3)
```

### Log startup dan shutdown (JSON, contoh)
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"starting","version":"grimoire v0.1.0 (commit 1a2b3c4, built 2026-10-20T08:00:00Z)","config":{"DatabaseURL":"postgres://grimoire:***@localhost:5432/grimoire","SessionSecret":"***","HTTPAddr":":8080"}}
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"listening","addr":"[::]:8080"}
{"time":"2026-10-20T08:05:00Z","level":"INFO","msg":"shutdown: http selesai"}
{"time":"2026-10-20T08:05:00Z","level":"INFO","msg":"shutdown: jobs berhenti"}
{"time":"2026-10-20T08:05:00Z","level":"INFO","msg":"shutdown: bus selesai"}
{"time":"2026-10-20T08:05:00Z","level":"INFO","msg":"shutdown: db ditutup"}
```

### HTTP yang diverifikasi smoke (bentuk dimiliki usecase 5/6)

`GET /healthz` -> 200
```json
{ "status": "ok" }
```

`GET /readyz` -> 200 (DB hidup)
```json
{ "status": "ok" }
```

`GET /readyz` -> 503 (DB mati)
```json
{ "status": "unavailable" }
```

`GET /api/does-not-exist` -> 404
```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

`POST /api/anything` tanpa `X-Requested-With` -> 403
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib untuk request mutasi", "details": null }, "trace_id": "0b1c9d2e-1f3a-4b7c-8e90-aa11bb22cc33" }
```

`GET /api/x` dengan cookie `grimoire_session=abc` sebelum T01 -> 500 (gagal tertutup, bus belum punya provider)
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "7d2e..." }
```

`GET /` -> 200 `text/html`, `Cache-Control: no-cache` (body `index.html` ter-embed). `GET /assets/missing.js` -> 404 teks polos.

Tidak ada body request/response baru untuk bus: usecase ini tidak mendefinisikan atau mengonsumsi kontrak bus.

## 9. Definition of Done

- `go build ./...`, `go vet ./...`, `go test -race ./...`, `make test-integration` hijau.
- `make build` menghasilkan satu file `bin/grimoire` yang lolos `TestT00_AC2_BuiltBinaryServesWithOnlyRequiredEnv` secara manual (`cd $(mktemp -d)` lalu jalankan dengan dua env).
- `cmd/http/main.go` dan `cmd/cli/main.go` tidak berisi logika selain memanggil `server.ServeFromEnv` / `newCLI(...).run` (rule 4.7).
- Tidak ada import `internal/domain/**` dari `internal/infra/**`; archtest lulus.
- Tidak ada env var baru; `TestT00_AC7_AllConfigFieldsDocumented` tetap hijau.
- Hanya file plan ini yang ditulis oleh perencana; implementasi mengikuti daftar Changes di atas.
