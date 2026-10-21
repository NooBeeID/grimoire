import { expect, test, type Browser } from '@playwright/test';
import {
  adminContext as adminContextIn,
  createReadyUserApi,
  loginUi,
  newContext as newContextIn,
  STRONG_PASSWORD,
  CSRF,
} from './helpers/auth';
import { createKeyApi, listKeysApi, revokeKeyApi, uniqueKeyName } from './helpers/apikeys';
import { createProjectApi } from './helpers/projects';

// E2E T04 project/api-key terhadap binary sungguhan (butuh backend T01 + T02 + T03 + T04).

const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

async function setup(browser: Browser) {
  const admin = await adminContext(browser);
  const owner = await createReadyUserApi(admin.request, { name: 'Pemilik E2E' });
  return { admin, owner };
}

test.describe('project api-key', () => {
  test('T04-AC1 anggota membuat key, T04-AC2 key hanya tampil sekali', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);
    const name = uniqueKeyName('claude');

    await page.goto(`/projects/${proj.id}/api-keys`);
    await expect(page.getByText('Belum ada API key').first()).toBeVisible();
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill(name);
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();

    await expect(page.getByRole('alert').filter({ hasText: 'tidak akan ditampilkan lagi' })).toBeVisible();
    const raw = await page.getByTestId('raw-key').inputValue();
    expect(raw).toMatch(/^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$/);
    const dialog = page.getByRole('dialog');
    const cli = dialog.locator('textarea').filter({ hasText: 'claude mcp add' });
    await expect(cli).toHaveValue(new RegExp(`^claude mcp add --transport http ${name} \\S+/mcp --header "Authorization: Bearer ${raw}"$`));
    const json = JSON.parse(await dialog.locator('textarea').filter({ hasText: 'mcpServers' }).inputValue());
    expect(json.mcpServers[name]).toMatchObject({ type: 'http', headers: { Authorization: `Bearer ${raw}` } });
    expect(json.mcpServers[name].url).toMatch(/\/mcp$/);

    await page.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();
    const row = page.getByRole('row').filter({ hasText: name });
    await expect(row).toContainText('Aktif');
    expect(await page.content()).not.toContain(raw);
    expect(page.url()).not.toContain(raw);

    const items = await listKeysApi(ctx.request, proj.id);
    expect(JSON.stringify(items)).not.toContain(raw);

    await ctx.close();
    await admin.close();
  });

  test('cabut key lalu daftar menampilkan status dicabut tanpa tombol hapus', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);
    const { key } = await createKeyApi(ctx.request, proj.id, uniqueKeyName('ci'));

    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: `Cabut key ${key.name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();

    const row = page.getByRole('row').filter({ hasText: key.name });
    await expect(row).toContainText('Dicabut');
    await expect(row.getByRole('button', { name: /Cabut/ })).toHaveCount(0);
    await expect(page.getByRole('button', { name: /Hapus/ })).toHaveCount(0);

    await page.reload();
    await expect(page.getByRole('row').filter({ hasText: key.name })).toContainText('Dicabut');
    await ctx.close();
    await admin.close();
  });

  test('bukan anggota mendapat 404 seragam; tanpa CSRF ditolak', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const outsider = await createReadyUserApi(admin.request, { name: 'Luar E2E' });
    const ownerCtx = await newContext(browser);
    const outCtx = await newContext(browser);
    const page = await outCtx.newPage();
    await loginUi(await ownerCtx.newPage(), owner.email, STRONG_PASSWORD);
    await loginUi(page, outsider.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ownerCtx.request);

    await page.goto(`/projects/${proj.id}/api-keys`);
    await expect(page.getByText('Project tidak ditemukan').first()).toBeVisible();
    expect((await outCtx.request.get(`/api/projects/${proj.id}/api-keys`)).status()).toBe(404);

    const noCsrf = await ownerCtx.request.post(`/api/projects/${proj.id}/api-keys`, { data: { name: 'tanpa-csrf' } });
    expect(noCsrf.status()).toBe(403);
    await ownerCtx.close();
    await outCtx.close();
    await admin.close();
  });

  test('T04-P4 nama sama dengan key yang sudah dicabut boleh dipakai lagi', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);
    const name = uniqueKeyName('ulang');
    const first = await createKeyApi(ctx.request, proj.id, name);
    await revokeKeyApi(ctx.request, proj.id, first.key.id);

    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill(name);
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByTestId('raw-key')).toBeVisible();
    await page.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();

    const rows = page.getByRole('row').filter({ hasText: name });
    await expect(rows).toHaveCount(2);
    await expect(rows.filter({ hasText: 'Aktif' })).toHaveCount(1);
    await expect(rows.filter({ hasText: 'Dicabut' })).toHaveCount(1);
    await ctx.close();
    await admin.close();
  });

  test('T04-P5 Super Admin non-anggota membuat dan mencabut key; tautan Audit Log hanya untuk Super Admin', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ownerCtx = await newContext(browser);
    await loginUi(await ownerCtx.newPage(), owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ownerCtx.request);
    const devKey = await createKeyApi(ownerCtx.request, proj.id, uniqueKeyName('dev'));

    const devPage = await ownerCtx.newPage();
    await devPage.goto(`/projects/${proj.id}/api-keys`);
    await expect(devPage.getByRole('row').filter({ hasText: devKey.key.name })).toBeVisible();
    await expect(devPage.getByRole('link', { name: /Audit Log/ })).toHaveCount(0);

    const page = await admin.newPage();
    const name = uniqueKeyName('admin');
    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill(name);
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByTestId('raw-key')).toBeVisible();
    await page.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();
    const row = page.getByRole('row').filter({ hasText: name });
    await expect(row).toContainText('Aktif');

    await page.getByRole('button', { name: `Cabut key ${name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();
    await expect(row).toContainText('Dicabut');

    const link = row.getByRole('link', { name: `Lihat Audit Log untuk key ${name}` });
    await expect(link).toBeVisible();
    const keyId = (await listKeysApi(admin.request, proj.id)).find((k) => k.name === name)!.id;
    expect(await link.getAttribute('href')).toContain(`target=${keyId}`);

    await expect(async () => {
      const res = await admin.request.get(`/api/audit-log?target=${keyId}&limit=50`);
      expect(res.status()).toBe(200);
      const actions = ((await res.json()) as { items: { action: string }[] }).items.map((i) => i.action);
      expect(actions).toEqual(expect.arrayContaining(['apikey.created', 'apikey.revoked']));
    }).toPass();

    await link.click();
    await expect(page).toHaveURL(/\/audit-log\?/);
    await expect(page).toHaveURL(new RegExp(`target=${keyId}`));
    await expect(page.getByText('apikey.created').first()).toBeVisible();
    await expect(page.getByText('apikey.revoked').first()).toBeVisible();

    await ownerCtx.close();
    await admin.close();
  });

  test('T04-N2 nama 1 karakter ditolak dengan pesan field name', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);

    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill('x');
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByRole('dialog').getByText('Nama key wajib 2-60 karakter')).toBeVisible();
    expect(await listKeysApi(ctx.request, proj.id)).toHaveLength(0);
    await ctx.close();
    await admin.close();
  });

  test('T04-N3 nama yang dipakai key aktif menampilkan NAME_TAKEN', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);
    const { key } = await createKeyApi(ctx.request, proj.id, uniqueKeyName('dup'));

    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill(key.name);
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByText('Nama ini sudah dipakai key aktif lain di project ini').first()).toBeVisible();
    await expect(page.getByTestId('raw-key')).toHaveCount(0);
    expect(await listKeysApi(ctx.request, proj.id)).toHaveLength(1);
    await ctx.close();
    await admin.close();
  });

  test('T04-N4 batas key aktif tercapai menampilkan KEY_LIMIT_REACHED dengan angka batas', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);

    // Isi sampai penuh lewat API; batas dibaca dari details respons 409 (default server 20).
    let limit = 0;
    for (let i = 0; i < 200 && limit === 0; i++) {
      const res = await ctx.request.post(`/api/projects/${proj.id}/api-keys`, { headers: CSRF, data: { name: `isi-${i}-${Date.now()}` } });
      if (res.status() === 409) {
        limit = ((await res.json()) as { error: { details: { limit: number } } }).error.details.limit;
      } else {
        expect(res.status(), await res.text()).toBe(201);
      }
    }
    expect(limit).toBeGreaterThan(0);

    await page.goto(`/projects/${proj.id}/api-keys`);
    await page.getByRole('button', { name: 'Buat key' }).first().click();
    await page.getByLabel('Nama key').fill(uniqueKeyName('lebih'));
    await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByText(`Batas ${limit} key aktif per project tercapai`).first()).toBeVisible();
    await ctx.close();
    await admin.close();
  });

  test('T04-N5 cabut key yang sudah dicabut di tab lain menampilkan ALREADY_REVOKED dan menyegarkan tabel', async ({ browser }) => {
    const { admin, owner } = await setup(browser);
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    await loginUi(page, owner.email, STRONG_PASSWORD);
    const proj = await createProjectApi(ctx.request);
    const { key } = await createKeyApi(ctx.request, proj.id, uniqueKeyName('tab'));

    await page.goto(`/projects/${proj.id}/api-keys`);
    const row = page.getByRole('row').filter({ hasText: key.name });
    await expect(row).toContainText('Aktif');

    // "Tab lain" mencabut lebih dulu; tab ini masih memegang data lama.
    await revokeKeyApi(ctx.request, proj.id, key.id);
    await page.getByRole('button', { name: `Cabut key ${key.name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();

    await expect(page.getByText('Key ini sudah dicabut. Daftar diperbarui.').first()).toBeVisible();
    await expect(row).toContainText('Dicabut');
    await expect(row.getByRole('button', { name: /Cabut/ })).toHaveCount(0);
    await ctx.close();
    await admin.close();
  });

  test('T04-N7 tanpa login GET daftar key mengembalikan 401', async ({ browser }) => {
    const ctx = await newContext(browser);
    const res = await ctx.request.get('/api/projects/01JB2X9K6Q4Y7V3N8M5T2R1C0A/api-keys');
    expect(res.status()).toBe(401);
    await ctx.close();
  });
});
