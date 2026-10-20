import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { ErrorEnvelopeForE2E } from './envelope';

// E2E T02 core/audit-log terhadap binary sungguhan.
// Bagian 1 berjalan sekarang (sisi penolakan tanpa sesi). Bagian 2 butuh login
// super admin (T01) dan data audit (T07 atau seed SQL), jadi ditandai fixme
// sampai helper di bawah diisi.

const ENTRY_ID = '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c';
const MUTATING = ['POST', 'PUT', 'PATCH', 'DELETE'] as const;
const CSRF = { 'X-Requested-With': 'grimoire' };

async function expectEnvelope(res: Awaited<ReturnType<APIRequestContext['get']>>, status: number, code: string) {
  expect(res.status()).toBe(status);
  expect(res.headers()['content-type']).toContain('json');
  const body = ErrorEnvelopeForE2E.parse(await res.json());
  expect(body.error.code).toBe(code);
  expect(body.trace_id).toBeTruthy();
}

test.describe('audit-log tanpa sesi', () => {
  test('GET list dan detail -> 401 UNAUTHORIZED (negatif 2)', async ({ request }) => {
    await expectEnvelope(await request.get('/api/audit-log'), 401, 'UNAUTHORIZED');
    await expectEnvelope(await request.get(`/api/audit-log/${ENTRY_ID}`), 401, 'UNAUTHORIZED');
    // validasi tidak bocor ke pemanggil anonim: auth dicek lebih dulu
    await expectEnvelope(await request.get('/api/audit-log?limit=500&action=BAD'), 401, 'UNAUTHORIZED');
    await expectEnvelope(await request.get('/api/audit-log/not-a-uuid'), 401, 'UNAUTHORIZED');
  });

  test('method mutasi tidak pernah berhasil (AC7, black-box)', async ({ request }) => {
    for (const method of MUTATING) {
      for (const path of ['/api/audit-log', `/api/audit-log/${ENTRY_ID}`]) {
        const noCsrf = await request.fetch(path, { method });
        await expectEnvelope(noCsrf, 403, 'CSRF_REQUIRED');
        const withCsrf = await request.fetch(path, { method, headers: CSRF });
        await expectEnvelope(withCsrf, 401, 'UNAUTHORIZED');
      }
    }
  });

  test('UI /audit-log anonim tidak menampilkan data dan tidak memanggil API', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    page.on('pageerror', (e) => errors.push(e.message));
    const apiCalls: string[] = [];
    page.on('request', (r) => new URL(r.url()).pathname.startsWith('/api/audit-log') && apiCalls.push(r.url()));

    const res = await page.goto('/audit-log');
    expect(res?.status()).toBe(200);
    await expect(page.getByRole('heading', { level: 1, name: 'Audit Log' })).toHaveCount(0);
    await expect(page.getByRole('table')).toHaveCount(0);
    await page.waitForTimeout(300);
    expect(apiCalls).toEqual([]);
    expect(errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Bagian 2: butuh sesi super admin + data. Isi dua helper ini setelah T01 selesai.

type Role = 'super_admin' | 'developer';

async function loginAs(_page: Page, _role: Role): Promise<void> {
  throw new Error('loginAs belum tersedia: menunggu T01 (core/auth). Login lewat UI/API lalu simpan cookie sesi.');
}

async function seedAuditEntries(_n: number, _opts: { sameTimestamp?: boolean } = {}): Promise<string[]> {
  throw new Error('seedAuditEntries belum tersedia: isi lewat INSERT SQL ke audit_log atau panggilan MCP (T07).');
}

test.describe('audit-log sebagai super admin', () => {
  test.fixme(true, 'menunggu T01 (login) dan data audit; lihat helper loginAs/seedAuditEntries');

  test('1. menu tampil, tabel urut terbaru, ikon agent vs manusia', async ({ page }) => {
    await seedAuditEntries(5);
    await loginAs(page, 'super_admin');
    await page.goto('/');
    await page.getByRole('link', { name: 'Audit Log' }).click();
    await expect(page).toHaveURL(/\/audit-log$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Audit Log' })).toBeVisible();
    const rows = page.getByRole('row');
    await expect(rows.nth(1)).toBeVisible();
  });

  test('2. filter tersinkron ke URL dan bertahan setelah reload', async ({ page }) => {
    await loginAs(page, 'super_admin');
    await page.goto('/audit-log?action=mcp.read_file&range=7d');
    await page.reload();
    await expect(page).toHaveURL(/action=mcp\.read_file/);
  });

  test('3. muat berikutnya memakai next_cursor tanpa duplikat; halaman terakhir tanpa tombol', async ({ page }) => {
    await seedAuditEntries(120, { sameTimestamp: true });
    await loginAs(page, 'super_admin');
    await page.goto('/audit-log');
    const ids = new Set<string>();
    page.on('response', async (r) => {
      if (new URL(r.url()).pathname === '/api/audit-log' && r.ok()) {
        for (const it of (await r.json()).items as { id: string }[]) {
          expect(ids.has(it.id)).toBe(false);
          ids.add(it.id);
        }
      }
    });
    const more = page.getByRole('button', { name: /muat/i });
    while (await more.isVisible()) await more.click();
    expect(ids.size).toBe(120);
  });

  test('4. detail memakai ?entry=<id>; args HTML tampil sebagai teks', async ({ page }) => {
    const [id] = await seedAuditEntries(1);
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?entry=${id}`);
    await expect(page.locator('img[onerror]')).toHaveCount(0);
  });

  test('5. deep link actor_type+actor_id terfilter ke key itu', async ({ page }) => {
    const [id] = await seedAuditEntries(1);
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${id}`);
    await expect(page.getByRole('table')).toBeVisible();
  });

  test('negatif 3. parameter tidak valid -> 400 INVALID_ARGUMENT dengan details.field', async ({ page }) => {
    await loginAs(page, 'super_admin');
    for (const q of ['limit=500', 'action=BAD', 'status=x', 'from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z', 'actor_id=abc', 'cursor=rusak']) {
      const res = await page.request.get(`/api/audit-log?${q}`);
      await expectEnvelope(res, 400, 'INVALID_ARGUMENT');
      expect(((await res.json()).error.details as { field?: string }).field).toBeTruthy();
    }
  });

  test('negatif 4. id bukan uuid atau tak ada -> 404 NOT_FOUND', async ({ page }) => {
    await loginAs(page, 'super_admin');
    await expectEnvelope(await page.request.get('/api/audit-log/not-a-uuid'), 404, 'NOT_FOUND');
    await expectEnvelope(await page.request.get(`/api/audit-log/${ENTRY_ID}`), 404, 'NOT_FOUND');
  });

  test('negatif 5. method mutasi -> 405 METHOD_NOT_ALLOWED', async ({ page }) => {
    await loginAs(page, 'super_admin');
    for (const method of MUTATING) {
      await expectEnvelope(await page.request.fetch('/api/audit-log', { method, headers: CSRF }), 405, 'METHOD_NOT_ALLOWED');
    }
  });
});

test.describe('audit-log sebagai developer', () => {
  test.fixme(true, 'menunggu T01 (login)');

  test('negatif 1. UI -> halaman tidak ditemukan; API -> 403 FORBIDDEN (AC5)', async ({ page }) => {
    await loginAs(page, 'developer');
    await page.goto('/audit-log');
    await expect(page.getByTestId('notfound-title')).toBeVisible();
    await expectEnvelope(await page.request.get('/api/audit-log'), 403, 'FORBIDDEN');
  });
});
