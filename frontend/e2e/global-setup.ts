import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';

const BIN = resolve(process.cwd(), '../bin/grimoire');
const ADMIN_EMAIL = 'e2e-admin@tim.id';
const DEFAULT_ADMIN_PASSWORD = 'e2e-admin-password-1';

/** Migrasi + admin e2e, idempoten. Dilewati bila biner tidak ada (server eksternal via E2E_BASE_URL). */
export default function globalSetup(): void {
  if (!existsSync(BIN)) {
    if (process.env.E2E_BASE_URL) return;
    throw new Error(`${BIN} tidak ditemukan; jalankan "make build" dulu`);
  }
  const migrate = spawnSync(BIN, ['migrate', 'up'], { env: process.env, encoding: 'utf8' });
  if (migrate.status !== 0) {
    throw new Error(`migrate up gagal (${migrate.status}): ${migrate.stderr}${migrate.stdout}`);
  }
  const admin = spawnSync(BIN, ['create-admin', '--email', ADMIN_EMAIL, '--name', 'E2E Admin'], {
    env: { ...process.env, CREATE_ADMIN_PASSWORD: process.env.E2E_ADMIN_PASSWORD ?? DEFAULT_ADMIN_PASSWORD },
    encoding: 'utf8',
  });
  if (admin.status === 0) return;
  if (admin.status === 1 && `${admin.stderr}${admin.stdout}`.includes('sudah terdaftar')) return;
  throw new Error(`create-admin gagal (${admin.status}): ${admin.stderr}${admin.stdout}`);
}
