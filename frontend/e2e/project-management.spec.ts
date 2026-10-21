import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { ErrorEnvelopeForE2E } from './envelope';
import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  CSRF,
  adminContext as adminContextIn,
  apiLogin,
  createReadyUserApi,
  loginUi,
  newContext as newContextIn,
  STRONG_PASSWORD,
} from './helpers/auth';
import { addMemberApi, createProjectApi, deleteProjectApi, uniqueProjectName } from './helpers/projects';

// E2E T03 project/management terhadap binary sungguhan (butuh backend T01 + T02 + T03).
// Tiap test membuat user dan project sendiri dengan nama unik.

async function devContext(browser: Browser, admin: BrowserContext, name = 'Dev E2E') {
  const user = await createReadyUserApi(admin.request, { name });
  const ctx = await newContext(browser);
  const page = await ctx.newPage();
  await loginUi(page, user.email, STRONG_PASSWORD);
  await expect(page.getByTestId('home-title')).toBeVisible();
  return { user, ctx, page };
}

// Salinan teks UI di spec ini berbahasa Indonesia; kunci locale agar tidak bergantung pada bahasa browser.
const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

const uniqueTag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;

async function loginApi(browser: Browser, email: string) {
  const ctx = await newContext(browser);
  await apiLogin(ctx.request, email, STRONG_PASSWORD);
  return ctx;
}

/** Buka dialog Tambah anggota, cari kandidat lewat combobox, lalu pilih dengan keyboard. */
async function pickCandidate(page: Page, term: string, optionText: string) {
  await page.getByRole('button', { name: 'Tambah anggota' }).first().click();
  const dlg = page.getByRole('dialog');
  const combo = dlg.getByRole('combobox');
  await combo.fill(term);
  await expect(dlg.getByRole('option', { name: new RegExp(optionText) })).toBeVisible();
  await combo.press('ArrowDown');
  await combo.press('Enter');
  await expect(dlg.getByTestId('selected-candidate')).toContainText(optionText);
  return dlg;
}

// Tombol "Keluar" juga ada di header aplikasi (logout); batasi ke konten utama.
const leaveButton = (page: Page) => page.locator('#main').getByRole('button', { name: 'Keluar', exact: true });
const rowOf = (page: Page, text: string) => page.getByRole('row').filter({ hasText: text });

test.describe('project management', () => {
  test('T03-AC1 buat project lalu masuk workspace sebagai Owner', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin, 'Ana E2E');
    const name = uniqueProjectName('Payments');
    await expect(page.getByText('Belum ada project')).toBeVisible();
    await page.getByRole('button', { name: 'Project baru' }).first().click();
    await page.getByLabel(/Nama project/).fill(name);
    await page.getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+\/members$/);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(name);
    await expect(page.getByText('(Anda)')).toBeVisible();
    await ctx.close();
    await admin.close();
  });

  test('negatif 6: nama tidak valid tidak mengirim request', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const posts: string[] = [];
    page.on('request', (r) => r.method() === 'POST' && r.url().endsWith('/api/projects') && posts.push(r.url()));
    await page.getByRole('button', { name: 'Project baru' }).first().click();
    await page.getByLabel(/Nama project/).fill('A');
    await page.getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page.getByText('Nama project wajib 2-100 karakter')).toBeVisible();
    expect(posts).toEqual([]);
    await ctx.close();
    await admin.close();
  });

  test('T03-AC1 isolasi: project orang lain dan bukan-uuid sama-sama tidak ditemukan', async ({ browser }) => {
    const admin = await adminContext(browser);
    const a = await createReadyUserApi(admin.request, { name: 'Owner A' });
    const aCtx = await newContext(browser);
    await aCtx.request.post('/api/auth/login', { headers: CSRF, data: { email: a.email, password: STRONG_PASSWORD } });
    const project = await createProjectApi(aCtx.request);
    const { ctx, page } = await devContext(browser, admin, 'Dev C');
    await page.goto(`/projects/${project.id}/members`);
    await expect(page.getByText('Project tidak ditemukan')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Kembali ke daftar project' })).toHaveAttribute('href', '/');
    await expect(page.getByText(project.name)).toHaveCount(0);
    await page.goto('/projects/bukan-uuid');
    await expect(page.getByText('Project tidak ditemukan')).toBeVisible();
    await ctx.close();
    await aCtx.close();
    await admin.close();
  });

  test('T03-AC2/AC3 member lewat API ditolak; owner terakhir tidak bisa keluar', async ({ browser }) => {
    const admin = await adminContext(browser);
    const a = await createReadyUserApi(admin.request, { name: 'Owner A' });
    const b = await createReadyUserApi(admin.request, { name: 'Member B' });
    const aCtx = await newContext(browser);
    await aCtx.request.post('/api/auth/login', { headers: CSRF, data: { email: a.email, password: STRONG_PASSWORD } });
    const project = await createProjectApi(aCtx.request);
    await addMemberApi(aCtx.request, project.id, b.id);

    const bCtx = await newContext(browser);
    await bCtx.request.post('/api/auth/login', { headers: CSRF, data: { email: b.email, password: STRONG_PASSWORD } });
    const add = await bCtx.request.post(`/api/projects/${project.id}/members`, { headers: CSRF, data: { user_id: a.id } });
    expect(add.status()).toBe(403);
    const del = await bCtx.request.delete(`/api/projects/${project.id}`, { headers: CSRF, data: { confirm_name: project.name } });
    expect(del.status()).toBe(403);

    const page = await aCtx.newPage();
    await page.goto(`/projects/${project.id}/members`);
    await leaveButton(page).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Keluar', exact: true }).click();
    await expect(page.getByRole('dialog').getByRole('alert')).toContainText('Owner terakhir');
    await page.reload();
    await expect(page.getByText('(Anda)')).toBeVisible();
    await aCtx.close();
    await bCtx.close();
    await admin.close();
  });

  test('T03-AC6 hapus project dengan mengetik nama persis', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const name = uniqueProjectName('Del');
    const project = await createProjectApi(ctx.request, name);
    await page.goto(`/projects/${project.id}/settings`);
    await page.getByRole('button', { name: 'Hapus project' }).click();
    const dlg = page.getByRole('dialog');
    const confirmBtn = dlg.getByRole('button', { name: 'Hapus permanen' });
    await dlg.getByLabel('Nama project').fill(name.toLowerCase());
    await expect(confirmBtn).toBeDisabled();
    await dlg.getByLabel('Nama project').fill(name);
    await confirmBtn.click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('link', { name })).toHaveCount(0);
    await ctx.close();
    await admin.close();
  });

  test('negatif 11: respons rusak menampilkan ErrorState tanpa crash', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    await page.route('**/api/projects?*', (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '{"items":"x"}' }),
    );
    await page.goto('/');
    await expect(page.getByText('Respons server tidak sesuai format')).toBeVisible();
    await ctx.close();
    await admin.close();
  });

  test('negatif 12: tanpa login diarahkan ke login lalu kembali ke workspace', async ({ browser }) => {
    const ctx = await newContext(browser);
    const page = await ctx.newPage();
    const id = '0192f5a0-1111-7aaa-8bbb-000000000001';
    await page.goto(`/projects/${id}`);
    await expect(page).toHaveURL(new RegExp(`/login\\?next=%2Fprojects%2F${id}`));
    await ctx.close();
  });

  test('negatif 13: mutasi tanpa header CSRF ditolak', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx } = await devContext(browser, admin);
    const res = await ctx.request.post('/api/projects', { data: { name: 'X Y' } });
    expect(res.status()).toBe(403);
    expect(ErrorEnvelopeForE2E.parse(await res.json()).error.code).toBe('CSRF_REQUIRED');
    await ctx.close();
    await admin.close();
  });

  test('negatif 14: nama project berisi HTML ditampilkan sebagai teks', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const name = '<img src=x onerror=window.__xss=1>';
    const project = await createProjectApi(ctx.request, name);
    await page.goto('/');
    await expect(page.getByRole('link', { name })).toBeVisible();
    await page.goto(`/projects/${project.id}/members`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(name);
    expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
    await ctx.close();
    await admin.close();
  });
  test('positif 2: tambah anggota lewat autocomplete dan keyboard', async ({ browser }) => {
    const admin = await adminContext(browser);
    const tag = uniqueTag();
    const b = await createReadyUserApi(admin.request, { name: `Bud${tag}` });
    const d = await createReadyUserApi(admin.request, { name: `Bud${tag}-off` });
    expect((await admin.request.post(`/api/users/${d.id}/deactivate`, { headers: CSRF })).status()).toBeLessThan(300);
    const { ctx, page } = await devContext(browser, admin, `Ana${tag}`);
    const project = await createProjectApi(ctx.request);
    await page.goto(`/projects/${project.id}/members`);

    await page.getByRole('button', { name: 'Tambah anggota' }).first().click();
    const dlg = page.getByRole('dialog');
    const combo = dlg.getByRole('combobox');
    await combo.fill(`Bud${tag}`);
    await expect(dlg.getByRole('option', { name: new RegExp(b.email) })).toBeVisible();
    await expect(dlg.getByRole('option', { name: new RegExp(d.email) })).toHaveCount(0);
    await expect(dlg.getByRole('option', { name: new RegExp(`Ana${tag}`) })).toHaveCount(0);
    await combo.press('ArrowDown');
    await combo.press('Enter');
    await expect(dlg.getByTestId('selected-candidate')).toContainText(b.email);
    await dlg.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(page.getByText(`${b.name} ditambahkan`)).toBeVisible();
    await expect(rowOf(page, b.email)).toContainText('Member');
    await ctx.close();
    await admin.close();
  });

  test('positif 3: member mengubah nama lewat UI tanpa zona bahaya dan hanya bisa keluar', async ({ browser }) => {
    const admin = await adminContext(browser);
    const tag = uniqueTag();
    const a = await createReadyUserApi(admin.request, { name: `Owner${tag}` });
    const { user: b, ctx: bCtx, page } = await devContext(browser, admin, `Member${tag}`);
    const aCtx = await loginApi(browser, a.email);
    const name = `Payments ${tag}`;
    const renamed = `${name} v2`;
    const project = await createProjectApi(aCtx.request, name);
    await addMemberApi(aCtx.request, project.id, b.id);

    await page.goto('/');
    const row = page.locator('li[data-project-id]').filter({ hasText: name });
    await expect(row).toContainText('Member');

    await row.getByRole('link', { name }).click();
    await page.getByRole('link', { name: 'Settings', exact: true }).click();
    await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+\/settings$/);
    await page.getByRole('textbox', { name: 'Nama project' }).fill(renamed);
    await page.getByRole('button', { name: 'Simpan' }).click();
    await expect(page.getByText('Perubahan disimpan')).toBeVisible();
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(renamed);
    await expect(page.getByText('Zona bahaya')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Hapus project' })).toHaveCount(0);

    await page.getByRole('link', { name: 'Members', exact: true }).click();
    await expect(rowOf(page, b.email)).toContainText('(Anda)');
    await expect(leaveButton(page)).toHaveCount(1);
    await expect(page.getByRole('button', { name: 'Jadikan owner' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Tambah anggota' })).toHaveCount(0);
    await bCtx.close();
    await aCtx.close();
    await admin.close();
  });

  test('positif 4: serah terima owner lalu pemilik lama keluar', async ({ browser }) => {
    const admin = await adminContext(browser);
    const a = await createReadyUserApi(admin.request, { name: 'Owner A' });
    const b = await createReadyUserApi(admin.request, { name: 'Member B' });
    const aCtx = await loginApi(browser, a.email);
    const project = await createProjectApi(aCtx.request, uniqueProjectName('Handover'));
    await addMemberApi(aCtx.request, project.id, b.id);

    const page = await aCtx.newPage();
    await page.goto(`/projects/${project.id}/members`);
    await rowOf(page, b.email).getByRole('button', { name: 'Jadikan owner' }).click();
    await expect(rowOf(page, b.email)).toContainText('Owner');
    await leaveButton(page).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Keluar', exact: true }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('link', { name: project.name })).toHaveCount(0);

    const bCtx = await loginApi(browser, b.email);
    const bPage = await bCtx.newPage();
    await bPage.goto(`/projects/${project.id}/members`);
    await expect(rowOf(bPage, b.email)).toContainText('Owner');
    await expect(bPage.getByRole('button', { name: 'Tambah anggota' }).first()).toBeVisible();
    await aCtx.close();
    await bCtx.close();
    await admin.close();
  });

  test('positif 5: Super Admin mengelola project orang lain dan melihat badge Bukan anggota', async ({ browser }) => {
    const admin = await adminContext(browser);
    const tag = uniqueTag();
    const a = await createReadyUserApi(admin.request, { name: `Alfa${tag}` });
    const c = await createReadyUserApi(admin.request, { name: `Cici${tag}` });
    const m = await createReadyUserApi(admin.request, { name: `Mira${tag}` });
    const aCtx = await loginApi(browser, a.email);
    const cCtx = await loginApi(browser, c.email);
    const aProject = await createProjectApi(aCtx.request, `Adm-A-${tag}`);
    const cProject = await createProjectApi(cCtx.request, `Adm-C-${tag}`);
    await addMemberApi(cCtx.request, cProject.id, m.id);

    const page = await admin.newPage();
    await page.goto(`/?q=${tag}`);
    await expect(page.getByRole('link', { name: aProject.name })).toBeVisible();
    await expect(page.getByRole('link', { name: cProject.name })).toBeVisible();
    await expect(page.getByText('Bukan anggota')).toHaveCount(2);

    await page.getByRole('link', { name: cProject.name }).click();
    await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+\/members$/);
    await expect(page.getByText('Bukan anggota').first()).toBeVisible();
    await page.getByRole('button', { name: `Keluarkan ${m.name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Keluarkan', exact: true }).click();
    await expect(rowOf(page, m.email)).toHaveCount(0);
    await rowOf(page, c.email).getByRole('button', { name: 'Jadikan member' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Jadikan member' }).click();
    await expect(rowOf(page, c.email)).not.toContainText('Owner');

    const dlg = await pickCandidate(page, `Alfa${tag}`, a.email);
    await dlg.getByLabel('Owner').check();
    await dlg.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(rowOf(page, a.email)).toContainText('Owner');
    await aCtx.close();
    await cCtx.close();
    await admin.close();
  });

  test('positif 6: Super Admin membuat project dan menunjuk owner', async ({ browser }) => {
    const admin = await adminContext(browser);
    const tag = uniqueTag();
    const dev = await createReadyUserApi(admin.request, { name: `Dewi${tag}` });
    const page = await admin.newPage();
    await page.goto('/');
    await page.getByRole('button', { name: 'Project baru' }).first().click();
    await expect(page.getByRole('dialog').getByText(/Super Admin Anda tidak otomatis menjadi anggota/)).toBeVisible();
    const name = uniqueProjectName('Adm');
    await page.getByLabel(/Nama project/).fill(name);
    await page.getByRole('button', { name: 'Buat', exact: true }).click();
    await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+\/members$/);
    await expect(page.getByText('Project ini belum punya anggota')).toBeVisible();

    await page.getByRole('button', { name: 'Tambah anggota' }).first().click();
    const dlg = page.getByRole('dialog');
    await dlg.getByRole('combobox').fill(`Dewi${tag}`);
    await dlg.getByRole('option', { name: new RegExp(dev.email) }).click();
    await dlg.getByLabel('Owner').check();
    await dlg.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(rowOf(page, dev.email)).toContainText('Owner');
    await admin.close();
  });

  test('positif 7: hapus project tercatat di audit log project.deleted', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const name = uniqueProjectName('Pay v2');
    const project = await createProjectApi(ctx.request, name);
    await page.goto(`/projects/${project.id}/settings`);
    await page.getByRole('button', { name: 'Hapus project' }).click();
    const dlg = page.getByRole('dialog');
    await expect(dlg.getByText(/Akan dihapus:/)).toBeVisible();
    await dlg.getByLabel('Nama project').fill(name);
    await dlg.getByRole('button', { name: 'Hapus permanen' }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByText(`Project "${name}" dihapus`)).toBeVisible();
    await expect(page.getByRole('link', { name })).toHaveCount(0);

    const adminPage = await admin.newPage();
    await adminPage.goto('/audit-log?action=project.deleted');
    await expect(adminPage.getByRole('heading', { level: 1, name: 'Audit Log' })).toBeVisible();
    await expect(adminPage.getByText(name).first()).toBeVisible();
    await ctx.close();
    await admin.close();
  });

  test('positif 8: pencarian dan paginasi 25 project tersinkron ke URL', async ({ browser }) => {
    test.setTimeout(90_000);
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const tag = uniqueTag();
    for (let i = 0; i < 25; i++) {
      await createProjectApi(ctx.request, `${tag}-${i < 3 ? 'alpha' : 'beta'}-${String(i).padStart(2, '0')}`);
    }
    const rows = page.locator('li[data-project-id]');
    await page.goto('/');
    await expect(rows).toHaveCount(20);
    await page.getByRole('button', { name: 'Berikutnya' }).click();
    await expect(page).toHaveURL(/\?page=2$/);
    await expect(rows).toHaveCount(5);

    await page.getByRole('searchbox', { name: 'Cari project' }).fill('alpha');
    await expect(page).toHaveURL(/\?q=alpha$/);
    await expect(rows).toHaveCount(3);
    await page.reload();
    await expect(page).toHaveURL(/\?q=alpha$/);
    await expect(rows).toHaveCount(3);

    await page.goBack();
    await expect(page).not.toHaveURL(/q=alpha/);
    await expect(rows).toHaveCount(20);
    await ctx.close();
    await admin.close();
  });

  test('negatif 3b: Jadikan member pada diri sendiri sebagai owner terakhir ditolak server', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);
    await page.goto(`/projects/${project.id}/members`);
    await page.getByRole('button', { name: 'Jadikan member' }).click();
    const dlg = page.getByRole('dialog');
    await dlg.getByRole('button', { name: 'Jadikan member' }).click();
    await expect(dlg.getByRole('alert')).toContainText('Owner terakhir');
    await page.reload();
    await expect(rowOf(page, '(Anda)')).toContainText('Owner');
    await ctx.close();
    await admin.close();
  });

  test('negatif 4: kandidat tidak layak ditolak lewat API dan pesan i18n di UI', async ({ browser }) => {
    const admin = await adminContext(browser);
    const d = await createReadyUserApi(admin.request, { name: 'Dev Nonaktif' });
    expect((await admin.request.post(`/api/users/${d.id}/deactivate`, { headers: CSRF })).status()).toBeLessThan(300);
    const adminUser = await apiLogin(admin.request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const b = await createReadyUserApi(admin.request, { name: `Bagus${uniqueTag()}` });
    const { ctx, page } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);

    const inactive = await ctx.request.post(`/api/projects/${project.id}/members`, { headers: CSRF, data: { user_id: d.id } });
    expect(inactive.status()).toBe(400);
    expect(ErrorEnvelopeForE2E.parse(await inactive.json()).error.details).toMatchObject({ reason: 'USER_INACTIVE' });
    const notDev = await ctx.request.post(`/api/projects/${project.id}/members`, { headers: CSRF, data: { user_id: adminUser.user.id } });
    expect(notDev.status()).toBe(400);
    expect(ErrorEnvelopeForE2E.parse(await notDev.json()).error.details).toMatchObject({ reason: 'USER_NOT_DEVELOPER' });

    await page.route(`**/api/projects/${project.id}/members`, (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 400,
            contentType: 'application/json',
            body: JSON.stringify({
              error: { code: 'INVALID_ARGUMENT', message: 'user nonaktif', details: { reason: 'USER_INACTIVE' } },
              trace_id: 'trace-e2e-1',
            }),
          })
        : route.fallback(),
    );
    await page.goto(`/projects/${project.id}/members`);
    const dlg = await pickCandidate(page, b.name, b.email);
    await dlg.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(dlg.getByRole('alert')).toContainText('User nonaktif tidak bisa ditambahkan');
    await ctx.close();
    await admin.close();
  });

  test('negatif 5: duplikat dari dua tab menampilkan ALREADY_MEMBER dan satu baris', async ({ browser }) => {
    const admin = await adminContext(browser);
    const tag = uniqueTag();
    const b = await createReadyUserApi(admin.request, { name: `Bima${tag}` });
    const { ctx, page: tab1 } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);
    const tab2 = await ctx.newPage();
    await tab1.goto(`/projects/${project.id}/members`);
    await tab2.goto(`/projects/${project.id}/members`);

    const dlg2 = await pickCandidate(tab2, `Bima${tag}`, b.email);
    const dlg1 = await pickCandidate(tab1, `Bima${tag}`, b.email);
    await dlg1.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(rowOf(tab1, b.email)).toHaveCount(1);
    await dlg2.getByRole('button', { name: 'Tambah', exact: true }).click();
    await expect(dlg2.getByRole('alert')).toContainText('User sudah menjadi anggota');
    await tab2.keyboard.press('Escape');
    await expect(rowOf(tab2, b.email)).toHaveCount(1);
    await ctx.close();
    await admin.close();
  });

  test('negatif 7: hapus dengan nama salah lewat API -> CONFIRMATION_MISMATCH', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);
    const res = await ctx.request.delete(`/api/projects/${project.id}`, { headers: CSRF, data: { confirm_name: `${project.name}x` } });
    expect(res.status()).toBe(400);
    expect(ErrorEnvelopeForE2E.parse(await res.json()).error.details).toMatchObject({ reason: 'CONFIRMATION_MISMATCH' });
    expect((await ctx.request.get(`/api/projects/${project.id}`)).status()).toBe(200);
    await ctx.close();
    await admin.close();
  });

  test('negatif 8: tab lama menyimpan Settings setelah project dihapus', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);
    await page.goto(`/projects/${project.id}/settings`);
    const nameInput = page.getByRole('textbox', { name: 'Nama project' });
    await expect(nameInput).toBeVisible();
    await deleteProjectApi(ctx.request, project.id, project.name);
    await nameInput.fill(`${project.name} baru`);
    await page.getByRole('button', { name: 'Simpan' }).click();
    await expect(page.getByText('Project tidak ditemukan')).toBeVisible();
    await expect(page).toHaveURL(/\/$/);
    await ctx.close();
    await admin.close();
  });

  test('negatif 9: akses dicabut saat tab terbuka lalu Keluar -> 404 dan ke beranda', async ({ browser }) => {
    const admin = await adminContext(browser);
    const a = await createReadyUserApi(admin.request, { name: 'Owner A' });
    const { user: b, ctx: bCtx, page: bPage } = await devContext(browser, admin, 'Member B');
    const aCtx = await loginApi(browser, a.email);
    const project = await createProjectApi(aCtx.request);
    await addMemberApi(aCtx.request, project.id, b.id);

    await bPage.goto(`/projects/${project.id}/members`);
    await expect(leaveButton(bPage)).toBeVisible();
    const rm = await aCtx.request.delete(`/api/projects/${project.id}/members/${b.id}`, { headers: CSRF });
    expect(rm.status()).toBeLessThan(300);
    await leaveButton(bPage).click();
    await bPage.getByRole('dialog').getByRole('button', { name: 'Keluar', exact: true }).click();
    await expect(bPage).toHaveURL(/\/$/);
    await expect(bPage.getByText(/Internal|NOT_FOUND|undefined/)).toHaveCount(0);
    await aCtx.close();
    await bCtx.close();
    await admin.close();
  });

  test('negatif 10: pencarian kandidat gagal menampilkan searchError', async ({ browser }) => {
    const admin = await adminContext(browser);
    const { ctx, page } = await devContext(browser, admin);
    const project = await createProjectApi(ctx.request);
    await page.route('**/member-candidates*', (route) =>
      route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'INTERNAL', message: 'boom', details: null }, trace_id: 'trace-e2e-2' }),
      }),
    );
    await page.goto(`/projects/${project.id}/members`);
    await page.getByRole('button', { name: 'Tambah anggota' }).first().click();
    const dlg = page.getByRole('dialog');
    await dlg.getByRole('combobox').fill('bud');
    await expect(dlg.getByText('Pencarian gagal. Coba lagi.')).toBeVisible();
    await expect(dlg.getByText('Tidak ada developer aktif yang cocok')).toHaveCount(0);
    await ctx.close();
    await admin.close();
  });
});
