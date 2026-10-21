# T00 — Foundation (infra)

| | |
|---|---|
| Domain / Module | infra / foundation (bukan module domain) |
| Ukuran | L |
| Bergantung pada | – |
| Dibutuhkan oleh | semua tiket |

## Ringkasan

Kerangka yang dipakai semua module: struktur repo, bus in-process, koneksi DB dan migrasi, server HTTP dengan middleware, penyajian static Svelte yang di-embed ke binary Go, konfigurasi, CLI, dan scaffold frontend. Tanpa tiket ini, module lain tidak bisa dimulai.

## Tujuan

- Satu binary Go (`cmd/http`) yang menjalankan API, MCP, dan menyajikan UI Svelte. Satu Postgres sebagai satu-satunya dependensi eksternal.
- Bus yang mewajibkan interaksi lintas module/domain lewat kontrak netral.
- Batas import ditegakkan otomatis, bukan hanya konvensi.

## Non-goals

- Tidak ada RabbitMQ, Redis, worker, atau cron terpisah.
- Tidak ada fitur bisnis (auth, project, dsb.) di tiket ini.
- Tidak ada multi-tenant routing.

## Persyaratan fungsional

### F1. Struktur repo
Struktur sesuai bagian "Konvensi arsitektur" di `README.md`: `backend/`, `frontend/`, `api/`, `docs/`, `makefile`, `docker-compose.dev.yml` (Postgres saja).

### F2. Bus (`infra/bus`)
- `Registry` peta `endpoint string → Handler`.
- `Register(endpoint, handler)`: panic jika endpoint terdaftar dua kali. Urutan registrasi tidak berpengaruh.
- `Call(ctx, endpoint, req)`: sinkron; error jika handler belum terdaftar.
- `Publish(ctx, endpoint, req)`: goroutine, context di-detach dari cancel parent tetapi `trace_id`, `user_id`, `api_key_id` ikut terbawa; error hanya di-log; panic di handler di-recover dan di-log.
- `CallTyped[T](ctx, b, endpoint, req) (T, error)`: helper generik; registry `nil` = error wiring, tanpa fallback.
- Bus mendukung shutdown yang menunggu goroutine `Publish` yang sedang berjalan (batas waktu, mis. 5 detik).
- Konstanta endpoint di `lib/constants/bus.go`; kontrak di `lib/contracts/<domain>/<module>/<nama>/`.

### F3. Database (`infra/db`)
- Koneksi Postgres dengan sqlx dan pool yang bisa dikonfigurasi.
- `BeginTx(ctx, func(tx) error)` untuk transaksi (commit jika nil, rollback jika error/panic).
- Runner migrasi SQL dari `migrations/{core,project}/` dengan nama `YYYYMMDDHHMMSSmmm_*.sql`; jejak versi di tabel `schema_migrations`; jalan lewat `cmd/cli migrate up` dan opsional otomatis saat start (flag env).
- Migrasi bersifat forward-only; setiap file idempotent terhadap rerun gagal di tengah (dibungkus transaksi bila memungkinkan).

### F4. Server HTTP (`infra/http`, `infra/middleware`)
Middleware berurutan: recover → trace id (`X-Request-Id`, dibuat jika tidak ada) → access log terstruktur → security headers → rate limiter (penyedia limiter di sini, aturan per route didefinisikan module) → auth session (memanggil `core.auth.resolve-session` lewat bus) → role guard.
- Format error JSON seragam: `{ "error": { "code": "...", "message": "...", "details": ... }, "trace_id": "..." }`.
- Endpoint `/healthz` (proses hidup) dan `/readyz` (DB terjangkau).
- Penutupan bertahap (graceful shutdown) dengan menunggu request dan bus.
- Batas ukuran body dan timeout default, dapat diatur per route (upload butuh batas lebih besar).
- Tidak ada CORS (satu origin). Validasi header `Origin` untuk `/mcp` disediakan helper di sini, aturannya di T07.

### F5. Penyajian static Svelte (`infra/static`)
- Hasil build Svelte di-embed (`go:embed`) ke binary.
- Fallback SPA: path non-`/api`, non-`/mcp`, non-file → `index.html`.
- Cache header: aset ber-hash `immutable`, `index.html` `no-cache`.
- Mode dev: proxy ke Vite dev server agar tidak perlu rebuild.

### F6. Konfigurasi (`infra/config`)
Env var, divalidasi saat start, gagal cepat dengan pesan jelas. Minimal: `DATABASE_URL`, `HTTP_ADDR`, `SESSION_SECRET`/kunci cookie, `COOKIE_SECURE`, `AUDIT_RETENTION_DAYS` (default 90), batas upload (`MAX_FILE_BYTES`, `MAX_UPLOAD_BYTES`, `MAX_UPLOAD_FILES`), batas respons MCP, `RUN_MIGRATION`. Dokumentasikan semuanya di `docs/configuration.md`.

### F7. Job terjadwal (`infra/jobs`)
Scheduler ringan di dalam proses (interval/harian) dengan penguncian agar aman bila lebih dari satu instance (Postgres advisory lock). Module mendaftarkan job-nya (dipakai T01 untuk sesi kedaluwarsa dan T02 untuk retensi audit).

### F8. CLI (`cmd/cli`)
Subcommand: `migrate up`, `create-admin --email --name` (password dibaca interaktif atau dari stdin/env; isi pembuatan ada di T01), `version`.

### F9. Penegakan batas module
- Lint/tes arsitektur di CI yang gagal jika: domain mengimpor domain lain, module mengimpor module lain, consumer mengimpor package provider (hanya boleh `lib/constants`, `lib/contracts/...`, `infra/bus`), atau `lib/contracts` mengimpor domain/infra.
- Cek kesegaran `api/openapi.yml` (`make api-check`) dan bundle (`make api-bundle`).

### F10. Scaffold frontend
- Vite + Svelte + TypeScript + Tailwind; struktur `app/`, `domain/`, `shared/` sesuai README.
- `shared/services/http.ts`: klien fetch (cookie session, CSRF header, normalisasi error, redirect ke login saat 401).
- Router, layout kosong, guard route berdasarkan role, i18n minimal (id/en), komponen dasar (button, input, dialog, toast, tabel, pagination).
- `make build` membuat bundle Svelte lalu binary Go yang meng-embed-nya.

### F11. CSRF
Karena memakai cookie session, semua request mutasi (`POST/PUT/PATCH/DELETE`) di `/api` wajib header khusus (mis. `X-Requested-With` atau token) selain SameSite=Lax. `/mcp` tidak memakai cookie sehingga tidak terkena.

## Kriteria penerimaan

1. `make dev` menjalankan Postgres + backend + Vite; UI kosong terbuka di browser.
2. `make build` menghasilkan satu binary; menjalankannya dengan `DATABASE_URL` menyajikan UI dan `/healthz` tanpa file lain.
3. Tes unit bus: duplikasi register → panic; Call ke endpoint belum terdaftar → error; Publish membawa `trace_id`/`user_id`; panic di handler Publish tidak menjatuhkan proses.
4. Tes arsitektur gagal bila ada impor lintas module dan lulus pada repo bersih.
5. Migrasi dapat dijalankan dua kali tanpa efek samping.
6. Request mutasi tanpa header CSRF ditolak 403.
7. `docs/configuration.md` mencantumkan seluruh env var beserta default.

## Pertanyaan terbuka

Tidak ada yang menghalangi. Pemilihan library (router, migrasi, logger, rate limiter) diserahkan ke builder; pilih yang aktif dipelihara.
