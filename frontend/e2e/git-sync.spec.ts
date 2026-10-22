import { expect, request as pwRequest, test, type Browser, type Locator, type Page } from '@playwright/test';
import { setRepo, VALID_TOKEN } from './fixtures/fake-git-server';
import { adminContext as adminContextIn, apiLogin, createReadyUserApi, CSRF, loginUi, newContext as newContextIn, STRONG_PASSWORD } from './helpers/auth';
import {
  apiSpecsUrl,
  billingYaml,
  docsRepoFiles,
  getSyncConfigApi,
  INVALID_TOKEN,
  openSyncDialog,
  paymentsJson,
  repoUrl,
  rerunButton,
  reportRow,
  runSyncApi,
  saveTokenApi,
  syncButton,
  uniqueRepo,
  urlInput,
} from './helpers/git-sync';
import { getFileApi, openKnowledge, uploadApi } from './helpers/knowledge';
import { addMemberApi, createProjectApi } from './helpers/projects';

// E2E T14 core/git-credential + project/git-sync terhadap binary sungguhan dan server GitHub palsu
// (e2e/fixtures/fake-git-server.ts, dijalankan playwright.config.ts). Tiap tes memakai repo unik.

const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

async function setup(browser: Browser, opts: { token?: boolean } = {}) {
  const admin = await adminContext(browser);
  const owner = await createReadyUserApi(admin.request, { name: 'Rina Pemilik' });
  const ctx = await newContext(browser);
  const page = await ctx.newPage();
  await loginUi(page, owner.email, STRONG_PASSWORD);
  await expect(page).not.toHaveURL(/\/login/);
  const proj = await createProjectApi(ctx.request);
  if (opts.token !== false) await saveTokenApi(ctx.request);
  const repo = uniqueRepo();
  return { admin, owner, ctx, page, proj, repo };
}

// Dialog punya tombol X (aria-label Tutup) di header dan tombol Tutup di footer; pakai yang di footer.
const closeBtn = (dialog: Locator) => dialog.getByRole('button', { name: 'Tutup', exact: true }).last();
const summaryCount = (dialog: Locator, key: string) => dialog.getByTestId('report-summary').locator(`[data-key="${key}"]`);
const tokenRow = (page: Page) => page.getByRole('row').filter({ hasText: 'GitHub' });

const tree = (page: Page) => page.getByRole('tree', { name: 'Pohon file knowledge' });
const item = (page: Page, name: string) => tree(page).getByRole('treeitem').filter({ has: page.getByText(name, { exact: true }) });

test.describe('git sync', () => {
  test('1 simpan token sah di profil: pil Tersimpan, token tidak bocor', async ({ browser }) => {
    const { admin, ctx, page } = await setup(browser, { token: false });
    const leaked: string[] = [];
    page.on('response', async (res) => {
      if (!res.url().includes('/api/')) return;
      try {
        if ((await res.text()).includes(VALID_TOKEN)) leaked.push(res.url());
      } catch {
        // respons tanpa body
      }
    });

    await page.goto('/profile');
    await expect(page.getByRole('heading', { name: 'Token Git' })).toBeVisible();
    await expect(tokenRow(page)).toContainText('Belum ada');

    await tokenRow(page).getByRole('button', { name: 'Simpan token GitHub untuk github.com' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Token', { exact: true }).fill(VALID_TOKEN);
    await dialog.getByRole('button', { name: 'Simpan', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(tokenRow(page)).toContainText('Tersimpan');
    await expect(tokenRow(page)).toContainText('…7Qx2');
    expect(await page.content()).not.toContain(VALID_TOKEN);
    expect(leaked).toEqual([]);

    await ctx.close();
    await admin.close();
  });

  test('2 sync knowledge pertama: dibuat, dilewati, pohon terisi', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo, '/tree/main/docs'));
    await syncButton(dialog).click();

    await expect(reportRow(dialog, 'docs/auth/login-flow.md')).toContainText('Dibuat');
    await expect(reportRow(dialog, 'docs/api/rate-limit.md')).toContainText('Dibuat');
    await expect(reportRow(dialog, 'docs/img/diagram.png')).toContainText('Dilewati');
    await expect(reportRow(dialog, 'docs/img/diagram.png')).toContainText(/didukung/);
    await expect(summaryCount(dialog, 'created')).toHaveText('2');
    await expect(summaryCount(dialog, 'skipped')).toHaveText('1');
    await closeBtn(dialog).click();

    await expect(item(page, 'auth')).toBeVisible();
    await item(page, 'auth').click();
    await expect(item(page, 'login-flow.md')).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'auth/login-flow.md')).status).toBe(200);

    await ctx.close();
    await admin.close();
  });

  test('3 buka lagi: form terisi, Sync ulang tanpa perubahan', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    const first = await runSyncApi(ctx.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(first.status, JSON.stringify(first.body)).toBe(200);
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await expect(urlInput(dialog)).toHaveValue(/\/tree\/main\/docs$/);
    await expect(dialog.getByLabel('Branch')).toHaveValue('main');
    await expect(dialog.getByLabel('Folder di repo')).toHaveValue('docs');
    const summary = dialog.getByTestId('config-summary');
    await expect(summary).toContainText('Berhasil');
    await expect(summary).toContainText(/2 dibuat/);

    await rerunButton(dialog).click();
    await expect(summaryCount(dialog, 'unchanged')).toHaveText('2');
    await expect(summaryCount(dialog, 'created')).toHaveText('0');
    await expect(reportRow(dialog, 'docs/auth/login-flow.md')).toContainText('Tidak berubah');

    await ctx.close();
    await admin.close();
  });

  test('4 perubahan repo: diperbarui, file manual tetap ada', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    await uploadApi(ctx.request, proj.id, [{ path: 'notes/manual.md', content: '# Catatan manual\n' }]);
    const first = await runSyncApi(ctx.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(first.status, JSON.stringify(first.body)).toBe(200);

    await setRepo(repo, { files: { ...docsRepoFiles(), 'docs/auth/login-flow.md': '# Alur login v2\n\nTambah langkah verifikasi.\n' } });
    await openKnowledge(page, proj.id);
    const dialog = await openSyncDialog(page);
    await rerunButton(dialog).click();

    await expect(reportRow(dialog, 'docs/auth/login-flow.md')).toContainText('Diperbarui');
    await expect(summaryCount(dialog, 'updated')).toHaveText('1');
    expect((await getFileApi(ctx.request, proj.id, 'notes/manual.md')).status).toBe(200);
    const updated = await getFileApi(ctx.request, proj.id, 'auth/login-flow.md');
    expect(updated.body?.content).toContain('v2');

    await ctx.close();
    await admin.close();
  });

  test('5 sync API spec: billing dibuat, payments ditolak dengan pointer', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: { 'api/openapi/billing.yaml': billingYaml(), 'api/openapi/payments.json': paymentsJson() } });
    await page.goto(apiSpecsUrl(proj.id));

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo));
    await dialog.getByLabel('Branch').fill('main');
    await dialog.getByLabel('Folder di repo').fill('api/openapi');
    await syncButton(dialog).click();

    const billing = reportRow(dialog, 'api/openapi/billing.yaml');
    await expect(billing).toContainText('Dibuat');
    await expect(billing).toContainText('billing');
    await expect(billing).toContainText('14 endpoint');
    const payments = reportRow(dialog, 'api/openapi/payments.json');
    await expect(payments).toContainText('Ditolak');
    await payments.getByRole('button', { name: 'Tampilkan detail' }).click();
    await expect(dialog.locator('code', { hasText: '#/paths/~1charges/get' })).toBeVisible();
    await expect(dialog.locator('code', { hasText: 'EXTERNAL_REF' })).toBeVisible();
    await expect(dialog.getByText('Berhasil sebagian')).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('6 prefix "Simpan di folder" menaruh file di guides/', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo, '/tree/main/docs'));
    await dialog.getByLabel('Simpan di folder').fill('guides');
    await syncButton(dialog).click();

    await expect(reportRow(dialog, 'docs/auth/login-flow.md')).toContainText('guides/auth/login-flow.md');
    expect((await getFileApi(ctx.request, proj.id, 'guides/auth/login-flow.md')).status).toBe(200);
    expect((await getFileApi(ctx.request, proj.id, 'auth/login-flow.md')).status).toBe(404);

    await ctx.close();
    await admin.close();
  });
});

test.describe('git sync: negatif', () => {
  test('1 token tidak sah ditolak, status tetap Belum ada', async ({ browser }) => {
    const { admin, ctx, page } = await setup(browser, { token: false });
    await page.goto('/profile');

    await tokenRow(page).getByRole('button', { name: 'Simpan token GitHub untuk github.com' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Token', { exact: true }).fill(INVALID_TOKEN);
    await dialog.getByRole('button', { name: 'Simpan', exact: true }).click();

    await expect(dialog.getByText('Token tidak valid atau tidak punya akses')).toBeVisible();
    await dialog.getByRole('button', { name: 'Batal', exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(tokenRow(page)).toContainText('Belum ada');

    await ctx.close();
    await admin.close();
  });

  test('2 hapus token: dialog Sync terkunci, API CREDENTIAL_MISSING', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    const first = await runSyncApi(ctx.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(first.status, JSON.stringify(first.body)).toBe(200);

    await page.goto('/profile');
    await expect(tokenRow(page)).toContainText('Tersimpan');
    await tokenRow(page).getByRole('button', { name: 'Hapus token GitHub untuk github.com' }).click();
    const confirm = page.getByRole('dialog');
    await confirm.getByRole('button', { name: 'Hapus', exact: true }).click();
    await expect(confirm).toBeHidden();
    await expect(tokenRow(page)).toContainText('Belum ada');

    await openKnowledge(page, proj.id);
    const dialog = await openSyncDialog(page);
    await expect(dialog.getByTestId('credential-notice')).toBeVisible();
    await expect(dialog.getByRole('link', { name: 'Atur Token Git' })).toBeVisible();
    await expect(syncButton(dialog)).toBeDisabled();

    const res = await runSyncApi(ctx.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(res.status).toBe(422);
    expect(res.body.error.code).toBe('CREDENTIAL_MISSING');

    await ctx.close();
    await admin.close();
  });

  test('3 anggota tanpa token: config terlihat, Sync nonaktif, config tidak berubah', async ({ browser }) => {
    const { admin, ctx, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    const first = await runSyncApi(ctx.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(first.status, JSON.stringify(first.body)).toBe(200);
    const before = await getSyncConfigApi(ctx.request, proj.id);

    const userB = await createReadyUserApi(admin.request, { name: 'Budi Anggota' });
    await addMemberApi(ctx.request, proj.id, userB.id);
    const ctxB = await newContext(browser);
    const pageB = await ctxB.newPage();
    await loginUi(pageB, userB.email, STRONG_PASSWORD);
    await expect(pageB).not.toHaveURL(/\/login/);

    await openKnowledge(pageB, proj.id);
    const dialog = await openSyncDialog(pageB);
    await expect(dialog.getByTestId('config-summary')).toContainText(repo);
    await expect(dialog.getByTestId('credential-notice')).toBeVisible();
    await expect(syncButton(dialog)).toBeDisabled();

    const res = await runSyncApi(ctxB.request, proj.id, { kind: 'knowledge', url: repoUrl(repo, '/tree/main/docs') });
    expect(res.status).toBe(422);
    expect(res.body.error.code).toBe('CREDENTIAL_MISSING');
    const after = await getSyncConfigApi(ctx.request, proj.id);
    expect(after.body.config).toEqual(before.body.config);

    await ctxB.close();
    await ctx.close();
    await admin.close();
  });

  test('4 host bitbucket: HOST_NOT_ALLOWED tampil di form', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill('https://bitbucket.org/x/y');
    await dialog.getByLabel('Branch').fill('main');
    await syncButton(dialog).click();

    await expect(dialog.getByText(/Host Git tidak diizinkan/)).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('5 tanpa branch: pesan "Isi Branch"', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo));
    await syncButton(dialog).click();

    await expect(dialog.getByText(/isi Branch/i)).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('6 folder tidak ada: GIT_NOT_FOUND, ringkasan config Gagal', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles() });
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo, '/tree/main/tidak-ada'));
    await syncButton(dialog).click();
    await expect(dialog.getByText(/Folder tidak-ada tidak ditemukan/)).toBeVisible();
    await dialog.getByRole('button', { name: 'Batal', exact: true }).click();
    await expect(dialog).toBeHidden();

    const again = await openSyncDialog(page);
    await expect(again.getByTestId('config-summary')).toContainText('Gagal');

    await ctx.close();
    await admin.close();
  });

  test('7 rate limit provider: pesan membatasi permintaan', async ({ browser }) => {
    const { admin, ctx, page, proj, repo } = await setup(browser);
    await setRepo(repo, { files: docsRepoFiles(), rateLimit: true });
    await openKnowledge(page, proj.id);

    const dialog = await openSyncDialog(page);
    await urlInput(dialog).fill(repoUrl(repo, '/tree/main/docs'));
    await syncButton(dialog).click();

    await expect(dialog.getByTestId('sync-error')).toHaveAttribute('data-code', 'RATE_LIMITED');
    await expect(dialog.getByText(/Provider Git membatasi permintaan/)).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('8 non-anggota 404, tanpa login 401', async ({ browser, baseURL }) => {
    const { admin, ctx, proj } = await setup(browser);
    const outsider = await createReadyUserApi(admin.request, { name: 'Citra Luar' });
    const outsiderCtx = await newContext(browser);
    await apiLogin(outsiderCtx.request, outsider.email, STRONG_PASSWORD);

    const denied = await outsiderCtx.request.get(`/api/projects/${proj.id}/git-sync`, { params: { kind: 'knowledge' } });
    expect(denied.status()).toBe(404);
    expect((await denied.json()).error.code).toBe('NOT_FOUND');

    const anon = await pwRequest.newContext({ baseURL });
    const unauth = await anon.get(`/api/projects/${proj.id}/git-sync`, { params: { kind: 'knowledge' }, headers: CSRF });
    expect(unauth.status()).toBe(401);

    await anon.dispose();
    await outsiderCtx.close();
    await ctx.close();
    await admin.close();
  });
});
