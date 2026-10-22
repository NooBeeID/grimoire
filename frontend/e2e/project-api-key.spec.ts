import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test';
import {
  ADMIN_EMAIL,
  adminContext as adminContextIn,
  createReadyUserApi,
  loginUi,
  newContext as newContextIn,
  STRONG_PASSWORD,
  CSRF,
} from './helpers/auth';
import { createKeyApi, listKeysApi, revokeKeyApi, revokeKeyRawApi, uniqueKeyName } from './helpers/apikeys';
import { addMemberApi, createProjectApi } from './helpers/projects';

// E2E T04 project/api-key terhadap binary sungguhan (butuh backend T01 + T02 + T03 + T04).

const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

async function setup(browser: Browser) {
  const admin = await adminContext(browser);
  const owner = await createReadyUserApi(admin.request, {
    name: 'Pemilik E2E',
  });
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
    await expect(page.getByText('Belum ada API key Anda').first()).toBeVisible();
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
    expect(json.mcpServers[name]).toMatchObject({
      type: 'http',
      headers: { Authorization: `Bearer ${raw}` },
    });
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
    const outsider = await createReadyUserApi(admin.request, {
      name: 'Luar E2E',
    });
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
    await expect(row).toContainText(ADMIN_EMAIL);

    await page.getByRole('button', { name: `Cabut key ${name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();
    await expect(row).toContainText('Dicabut');

    const link = row.getByRole('link', {
      name: `Lihat Audit Log untuk key ${name}`,
    });
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
    await expect(page.getByText('Anda sudah punya key aktif dengan nama ini').first()).toBeVisible();
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
      const res = await ctx.request.post(`/api/projects/${proj.id}/api-keys`, {
        headers: CSRF,
        data: { name: `isi-${i}-${Date.now()}` },
      });
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
    await expect(page.getByText(`Batas ${limit} key aktif per user tercapai`).first()).toBeVisible();
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

// ---------------------------------------------------------------------------
// T13: key milik user di dalam project (Ana = owner project, Budi = anggota).
// ---------------------------------------------------------------------------

async function setupTwoUsers(browser: Browser) {
  const admin = await adminContext(browser);
  const ana = await createReadyUserApi(admin.request, { name: 'Ana E2E' });
  const budi = await createReadyUserApi(admin.request, { name: 'Budi E2E' });
  const anaCtx = await newContext(browser);
  const budiCtx = await newContext(browser);
  const anaPage = await anaCtx.newPage();
  const budiPage = await budiCtx.newPage();
  await loginUi(anaPage, ana.email, STRONG_PASSWORD);
  await loginUi(budiPage, budi.email, STRONG_PASSWORD);
  const proj = await createProjectApi(anaCtx.request);
  await addMemberApi(anaCtx.request, proj.id, budi.id, 'member');
  const close = async () => {
    await anaCtx.close();
    await budiCtx.close();
    await admin.close();
  };
  return { admin, ana, budi, anaCtx, budiCtx, anaPage, budiPage, proj, close };
}

async function createKeyUi(page: Page, name: string) {
  await page.getByRole('button', { name: 'Buat key' }).first().click();
  await page.getByLabel('Nama key').fill(name);
  await page.getByRole('dialog').getByRole('button', { name: 'Buat', exact: true }).click();
}

/** Isi kuota lewat API sampai 409; mengembalikan batas dari details.limit. */
async function fillToLimit(ctx: BrowserContext, projectId: string): Promise<number> {
  for (let i = 0; i < 200; i++) {
    const res = await ctx.request.post(`/api/projects/${projectId}/api-keys`, {
      headers: CSRF,
      data: { name: `isi-${i}-${Date.now()}` },
    });
    if (res.status() === 409) {
      return ((await res.json()) as { error: { details: { limit: number } } }).error.details.limit;
    }
    expect(res.status(), await res.text()).toBe(201);
  }
  throw new Error('batas key tidak tercapai dalam 200 percobaan');
}

type ErrBody = { error: Record<string, unknown> };
const stripTrace = async (res: { json: () => Promise<unknown> }) => {
  const { error } = (await res.json()) as ErrBody;
  const { trace_id: _trace, ...rest } = error;
  return rest;
};

test.describe('project api-key per user (T13)', () => {
  test('T13-P1 Ana hanya melihat key miliknya tanpa kolom pemilik; T13-P2 Budi hanya melihat miliknya', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));
    const b = await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));

    await s.anaPage.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(s.anaPage.getByRole('row').filter({ hasText: a.key.name })).toBeVisible();
    await expect(s.anaPage.getByText(b.key.name)).toHaveCount(0);
    await expect(s.anaPage.getByText('Key di sini hanya milik Anda.').first()).toBeVisible();
    await expect(s.anaPage.getByRole('columnheader', { name: 'Dibuat oleh' })).toHaveCount(0);

    await s.budiPage.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(s.budiPage.getByRole('row').filter({ hasText: b.key.name })).toBeVisible();
    await expect(s.budiPage.getByText(a.key.name)).toHaveCount(0);
    const raw = JSON.stringify(await listKeysApi(s.budiCtx.request, s.proj.id));
    expect(raw).not.toContain(a.key.id);
    expect(raw).not.toContain(a.key.prefix);
    expect(raw).not.toContain(a.key.name);
    await s.close();
  });

  test('T13-P3 dua user boleh membuat key bernama sama', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const name = uniqueKeyName('shared');
    for (const page of [s.anaPage, s.budiPage]) {
      await page.goto(`/projects/${s.proj.id}/api-keys`);
      await createKeyUi(page, name);
      await expect(page.getByTestId('raw-key')).toBeVisible();
      await expect(page.getByText('Anda sudah punya key aktif dengan nama ini')).toHaveCount(0);
      await page.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();
      await expect(page.getByRole('row').filter({ hasText: name })).toContainText('Aktif');
    }
    await s.close();
  });

  test('T13-P4 Ana mencabut key lalu membuat ulang nama yang sama; daftar Budi tidak berubah', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));
    await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));
    const budiBefore = await listKeysApi(s.budiCtx.request, s.proj.id);

    await s.anaPage.goto(`/projects/${s.proj.id}/api-keys`);
    await s.anaPage.getByRole('button', { name: `Cabut key ${a.key.name}` }).click();
    await s.anaPage.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();
    const rows = s.anaPage.getByRole('row').filter({ hasText: a.key.name });
    await expect(rows).toContainText('Dicabut');
    expect(await listKeysApi(s.budiCtx.request, s.proj.id)).toEqual(budiBefore);

    await createKeyUi(s.anaPage, a.key.name);
    await expect(s.anaPage.getByTestId('raw-key')).toBeVisible();
    await s.anaPage.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();
    await expect(rows).toHaveCount(2);
    await expect(rows.filter({ hasText: 'Aktif' })).toHaveCount(1);
    await expect(rows.filter({ hasText: 'Dicabut' })).toHaveCount(1);
    await s.close();
  });

  test('T13-P5 Super Admin melihat key semua anggota dengan kolom Dibuat oleh', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));
    const b = await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));

    const page = await s.admin.newPage();
    await page.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(page.getByRole('columnheader', { name: 'Dibuat oleh' })).toBeVisible();
    await expect(page.getByText('Sebagai Super Admin Anda melihat key semua anggota project.').first()).toBeVisible();
    await expect(page.getByRole('row').filter({ hasText: a.key.name })).toContainText(s.ana.email);
    await expect(page.getByRole('row').filter({ hasText: b.key.name })).toContainText(s.budi.email);
    await s.close();
  });

  test('T13-P6 Super Admin mencabut key Budi; Budi melihatnya dicabut dan audit tercatat', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const b = await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));

    const page = await s.admin.newPage();
    await page.goto(`/projects/${s.proj.id}/api-keys`);
    await page.getByRole('button', { name: `Cabut key ${b.key.name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cabut key' }).click();
    const row = page.getByRole('row').filter({ hasText: b.key.name });
    await expect(row).toContainText('Dicabut');
    await expect(row).toContainText(`Dicabut oleh ${ADMIN_EMAIL}`);

    await s.budiPage.goto(`/projects/${s.proj.id}/api-keys`);
    const budiRow = s.budiPage.getByRole('row').filter({ hasText: b.key.name });
    await expect(budiRow).toContainText('Dicabut');
    await expect(budiRow).toContainText(`Dicabut oleh ${ADMIN_EMAIL}`);

    await expect(async () => {
      const res = await s.admin.request.get(`/api/audit-log?target=${b.key.id}&limit=50`);
      expect(res.status()).toBe(200);
      const actions = ((await res.json()) as { items: { action: string }[] }).items.map((i) => i.action);
      expect(actions).toContain('apikey.revoked');
    }).toPass();
    await s.close();
  });

  test('T13-P7 key buatan Super Admin menampilkan emailnya dan tidak terlihat oleh Ana', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const name = uniqueKeyName('admin');
    const page = await s.admin.newPage();
    await page.goto(`/projects/${s.proj.id}/api-keys`);
    await createKeyUi(page, name);
    await expect(page.getByTestId('raw-key')).toBeVisible();
    await page.getByRole('button', { name: 'Saya sudah menyimpan key' }).click();
    await expect(page.getByRole('row').filter({ hasText: name })).toContainText(ADMIN_EMAIL);

    await s.anaPage.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(s.anaPage.getByText('Belum ada API key Anda').first()).toBeVisible();
    await expect(s.anaPage.getByText(name)).toHaveCount(0);
    await s.close();
  });

  test('T13-P8 kuota dihitung per user: Ana penuh, Budi masih bisa membuat', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const limit = await fillToLimit(s.anaCtx, s.proj.id);
    expect(limit).toBeGreaterThan(0);
    await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));
    await s.close();
  });

  test('T13-N1/N2/N3 revoke key orang lain, acak, atau bukan-uuid: 404 KEY_NOT_FOUND identik', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));

    const other = await revokeKeyRawApi(s.budiCtx.request, s.proj.id, a.key.id);
    expect(other.status()).toBe(404);
    const otherErr = await stripTrace(other);
    expect(otherErr).toMatchObject({
      code: 'NOT_FOUND',
      details: { reason: 'KEY_NOT_FOUND' },
    });
    const stillActive = (await listKeysApi(s.anaCtx.request, s.proj.id)).find((k) => k.id === a.key.id);
    expect(stillActive?.status).toBe('active');

    const random = await revokeKeyRawApi(s.budiCtx.request, s.proj.id, crypto.randomUUID());
    expect(random.status()).toBe(404);
    expect(await stripTrace(random)).toEqual(otherErr);

    const bad = await revokeKeyRawApi(s.budiCtx.request, s.proj.id, 'bukan-uuid');
    expect(bad.status()).toBe(404);
    expect(await stripTrace(bad)).toEqual(otherErr);
    await s.close();
  });

  test('T13-N4 nama sama dengan key aktif sendiri ditolak di dialog', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));
    await s.anaPage.goto(`/projects/${s.proj.id}/api-keys`);
    await createKeyUi(s.anaPage, a.key.name);
    await expect(s.anaPage.getByText('Anda sudah punya key aktif dengan nama ini').first()).toBeVisible();
    await expect(s.anaPage.getByTestId('raw-key')).toHaveCount(0);
    expect(await listKeysApi(s.anaCtx.request, s.proj.id)).toHaveLength(1);
    await s.close();
  });

  test('T13-N5 Ana mencapai batas lalu membuat lewat dialog menampilkan batas per user', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const limit = await fillToLimit(s.anaCtx, s.proj.id);
    await s.anaPage.goto(`/projects/${s.proj.id}/api-keys`);
    await createKeyUi(s.anaPage, uniqueKeyName('lebih'));
    await expect(s.anaPage.getByText(`Batas ${limit} key aktif per user tercapai`).first()).toBeVisible();
    await s.close();
  });

  test('T13-N6 user biasa tanpa kolom pemilik atau tautan Audit Log; /audit-log tidak tersedia', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const b = await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));
    await revokeKeyApi(s.budiCtx.request, s.proj.id, b.key.id);
    await createKeyApi(s.budiCtx.request, s.proj.id, uniqueKeyName('b-key'));

    await s.budiPage.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(s.budiPage.getByRole('row').filter({ hasText: b.key.name })).toBeVisible();
    await expect(s.budiPage.getByRole('columnheader', { name: 'Dibuat oleh' })).toHaveCount(0);
    await expect(s.budiPage.getByRole('link', { name: /Audit Log/ })).toHaveCount(0);

    await s.budiPage.goto('/audit-log');
    await expect(s.budiPage.getByTestId('notfound-title')).toBeVisible();
    expect((await s.budiCtx.request.get('/api/audit-log')).status()).toBe(403);
    await s.close();
  });

  test('T13-N8 Budi tanpa key melihat empty state dengan hint anggota lain', async ({ browser }) => {
    const s = await setupTwoUsers(browser);
    const a = await createKeyApi(s.anaCtx.request, s.proj.id, uniqueKeyName('a-key'));
    await s.budiPage.goto(`/projects/${s.proj.id}/api-keys`);
    await expect(s.budiPage.getByText('Belum ada API key Anda').first()).toBeVisible();
    await expect(s.budiPage.getByText('Key yang dibuat anggota lain tidak ditampilkan di sini.').first()).toBeVisible();
    await expect(s.budiPage.getByText(a.key.name)).toHaveCount(0);
    await expect(s.budiPage.getByRole('row').filter({ hasText: 'Aktif' })).toHaveCount(0);
    await s.close();
  });
});
