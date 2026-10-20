# Frontend dan static handler

## Mode dev
`make dev` menjalankan Postgres, backend (`:8080`), dan Vite (`:5173`). Buka `http://localhost:8080`: backend (APP_ENV=development, `DEV_VITE_URL` terisi) mem-proxy semua selain `/api`, `/mcp`, `/healthz`, `/readyz` ke Vite, termasuk websocket HMR. Cookie dan `/api` satu origin. Bila Vite mati, `/` membalas 502 dengan petunjuk.

## Build
`make build` menjalankan `npm ci && npm run build`, menyalin `frontend/dist` ke `backend/internal/infra/static/dist`, lalu `go build`. `make fe-build` hanya dua langkah pertama.

## Placeholder
`dist/index.html` placeholder dilacak git agar `go build` tanpa npm tetap jalan. Hasil build menimpanya; sebelum commit jalankan `make fe-restore-placeholder`.

## Aturan handler static
- `/api/**` dan `/mcp/**` selalu 404 JSON `NOT_FOUND`.
- Method selain GET/HEAD: 405 JSON, `Allow: GET, HEAD`.
- `/assets/*`: `Cache-Control: public, max-age=31536000, immutable`; aset hilang 404 teks polos.
- Path berekstensi yang hilang: 404; path lain: fallback `index.html` (`no-cache`).
- Proxy Vite hanya aktif bila `APP_ENV=development` dan `DEV_VITE_URL` terisi.

## Skrip frontend
`check`, `test`, `test:e2e` (butuh server di `E2E_BASE_URL`, default `http://localhost:8080`), `depcruise` dan `test:arch` (konfigurasi `.dependency-cruiser.cjs` milik architecture-guard). Fixture dependency-cruiser di `frontend/test/depcruise-fixtures` dikecualikan dari tsconfig dan vitest.
