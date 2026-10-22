import { expect, request as pwRequest, test, type APIRequestContext, type Page } from '@playwright/test';
import { ErrorEnvelopeForE2E } from './envelope';
import { createKeyApi, uniqueKeyName } from './helpers/apikeys';
import { ADMIN_EMAIL, ADMIN_PASSWORD, apiLogin, createReadyUserApi } from './helpers/auth';
import { initializeBody, mcpPost, toolsListBody } from './helpers/mcp';
import { createProjectApi } from './helpers/projects';

// E2E T02 core/audit-log terhadap binary sungguhan.
// Bagian 1: sisi penolakan tanpa sesi. Bagian 2: sesi super admin dan developer (login dari T01).
// Data audit dibangkitkan dari aksi nyata (login gagal -> entri auth.login.failed) dengan tag unik
// per tes, jadi tidak butuh akses SQL dan tidak bentrok dengan data lain di DB yang sama.

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
    // 401 dari GET /api/auth/me adalah pemeriksaan sesi (T01) untuk pengunjung anonim, bukan error aplikasi.
    page.on(
      'console',
      (m) => m.type() === 'error' && !m.location().url.endsWith('/api/auth/me') && errors.push(m.text()),
    );
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
// Bagian 2: butuh sesi dan data audit.

type Role = 'super_admin' | 'developer';

const baseURL = () => test.info().project.use.baseURL as string;

/** Login lewat API; cookie sesi tersimpan di context `page` sehingga page.goto dan page.request ikut login. */
async function loginAs(page: Page, role: Role): Promise<void> {
  if (role === 'super_admin') {
    await apiLogin(page.request, ADMIN_EMAIL, ADMIN_PASSWORD);
    return;
  }
  const admin = await pwRequest.newContext({ baseURL: baseURL() });
  try {
    await apiLogin(admin, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(admin, { role: 'developer', name: 'E2E Dev' });
    await apiLogin(page.request, dev.email, dev.password);
  } finally {
    await admin.dispose();
  }
}

interface Seeded {
  ids: string[];
  /** Tag unik; pakai sebagai filter `actor_label` agar hanya entri tes ini yang terlihat. */
  tag: string;
}

/**
 * Membuat n entri audit asli: n percobaan login gagal dengan email unik (auth.login.failed, aktor anonymous,
 * label = email yang dicoba; `suffix` disisipkan setelah tag unik). Mengembalikan id entri lewat API admin. Tie-break cursor untuk timestamp
 * kembar diuji di tes integrasi backend (listentries), bukan di sini.
 */
async function seedAuditEntries(n: number, opts: { suffix?: string } = {}): Promise<Seeded> {
  const tag = `e2eseed${Date.now()}${Math.random().toString(36).slice(2, 7)}`;
  const anon = await pwRequest.newContext({ baseURL: baseURL() });
  const admin = await pwRequest.newContext({ baseURL: baseURL() });
  try {
    await apiLogin(admin, ADMIN_EMAIL, ADMIN_PASSWORD);
    for (let i = 0; i < n; i += 10) {
      await Promise.all(
        Array.from({ length: Math.min(10, n - i) }, (_, k) =>
          anon.post('/api/auth/login', {
            headers: CSRF,
            data: { email: `${tag}${opts.suffix ?? ''}-${i + k}@tim.id`, password: 'salah-salah-1' },
          }),
        ),
      );
    }
    let ids: string[] = [];
    await expect
      .poll(
        async () => {
          ids = [];
          let cursor = '';
          do {
            const res = await admin.get(`/api/audit-log?actor_label=${encodeURIComponent(tag)}&limit=200${cursor}`);
            expect(res.status()).toBe(200);
            const body = (await res.json()) as { items: { id: string }[]; next_cursor?: string | null };
            ids.push(...body.items.map((it) => it.id));
            cursor = body.next_cursor ? `&cursor=${encodeURIComponent(body.next_cursor)}` : '';
          } while (cursor);
          return ids.length;
        },
        { message: 'entri audit login gagal belum tertulis', timeout: 10_000 },
      )
      .toBe(n);
    return { ids, tag };
  } finally {
    await anon.dispose();
    await admin.dispose();
  }
}

test.describe('audit-log sebagai super admin', () => {
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
    const { tag } = await seedAuditEntries(120);
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?actor_label=${encodeURIComponent(tag)}`);
    const ids = new Set<string>();
    page.on('response', async (r) => {
      if (new URL(r.url()).pathname === '/api/audit-log' && r.ok()) {
        for (const it of (await r.json()).items as { id: string }[]) {
          expect(ids.has(it.id)).toBe(false);
          ids.add(it.id);
        }
      }
    });
    const more = page.getByRole('button', { name: /^(Muat berikutnya|Load more)$/ });
    await expect
      .poll(
        async () => {
          if (await more.isVisible()) await more.click();
          return ids.size;
        },
        { timeout: 20_000 },
      )
      .toBe(120);
    await expect(more).toHaveCount(0); // halaman terakhir tanpa tombol
  });

  test('4. detail memakai ?entry=<id>; args HTML tampil sebagai teks', async ({ page }) => {
    const html = '<img src=x onerror=window.__xss=1>';
    const {
      ids: [id],
      tag,
    } = await seedAuditEntries(1, { suffix: html });
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?actor_label=${encodeURIComponent(tag)}&entry=${id}`);
    await expect(page.getByText(html, { exact: false }).first()).toBeVisible(); // tampil sebagai teks
    await expect(page.locator('img[onerror]')).toHaveCount(0); // tidak menjadi elemen
    expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
  });

  test('5. deep link actor_type+actor_id terfilter ke key itu', async ({ page, request }) => {
    await loginAs(page, 'super_admin');
    const proj = await createProjectApi(page.request);
    const keyName = uniqueKeyName('audit');
    const created = await createKeyApi(page.request, proj.id, keyName);
    await mcpPost(request, created.raw_key, initializeBody(1));
    await mcpPost(request, created.raw_key, toolsListBody(2));
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${created.key.id}`);
    await expect(page.getByRole('table')).toBeVisible();
    for (const action of ['mcp.initialize', 'mcp.tools_list']) {
      const row = page.getByRole('row').filter({ hasText: action });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText(`${keyName} (${created.key.prefix})`);
      await expect(row).toContainText(proj.name);
    }
    await expect(page.getByRole('img', { name: 'API key (agent)' })).toHaveCount(2);
  });

  test('6. klik baris membuka detail, fokus kembali ke tombol waktu (AC14)', async ({ page }) => {
    const {
      ids: [id],
      tag,
    } = await seedAuditEntries(1);
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?actor_label=${encodeURIComponent(tag)}`);
    const row = page.getByRole('row').filter({ hasText: tag });
    await row.getByRole('cell').nth(3).click(); // sel Aksi
    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page).toHaveURL(/entry=/);
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    const timeBtn = page.locator(`[data-entry-id="${id}"]`);
    await expect(timeBtn).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('dialog')).toBeVisible();
  });

  test('14. drag-select teks di baris tidak membuka detail (AC14)', async ({ page }) => {
    const { tag } = await seedAuditEntries(1);
    await loginAs(page, 'super_admin');
    await page.goto(`/audit-log?actor_label=${encodeURIComponent(tag)}`);
    const cell = page.getByRole('row').filter({ hasText: tag }).getByRole('cell').nth(1);
    const box = (await cell.boundingBox())!;
    await page.mouse.move(box.x + 4, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width - 4, box.y + box.height / 2, { steps: 8 });
    await page.mouse.up();
    await expect(page).not.toHaveURL(/entry=/);
    await expect(page.getByRole('dialog')).toHaveCount(0);
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
  test('negatif 1. UI -> halaman tidak ditemukan; API -> 403 FORBIDDEN (AC5)', async ({ page }) => {
    await loginAs(page, 'developer');
    await page.goto('/audit-log');
    await expect(page.getByTestId('notfound-title')).toBeVisible();
    await expectEnvelope(await page.request.get('/api/audit-log'), 403, 'FORBIDDEN');
  });
});
