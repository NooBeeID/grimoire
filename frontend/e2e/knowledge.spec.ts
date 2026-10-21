import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test, type Browser, type Locator, type Page } from '@playwright/test';
import { adminContext as adminContextIn, createReadyUserApi, CSRF, loginUi, newContext as newContextIn, STRONG_PASSWORD, submitLogin } from './helpers/auth';
import { getFileApi, openKnowledge, stageDocs, uploadApi, watchCsp } from './helpers/knowledge';
import { addMemberApi, createProjectApi } from './helpers/projects';

// E2E T05 project/knowledge terhadap binary sungguhan (butuh backend T01 + T02 + T03 + T05).

const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

async function setup(browser: Browser) {
  const admin = await adminContext(browser);
  const owner = await createReadyUserApi(admin.request, { name: 'Pemilik E2E' });
  const ctx = await newContext(browser);
  await ctx.grantPermissions(['clipboard-read', 'clipboard-write']);
  const page = await ctx.newPage();
  const csp = watchCsp(page);
  await loginUi(page, owner.email, STRONG_PASSWORD);
  await expect(page).not.toHaveURL(/\/login/); // sesi harus ada sebelum memakai ctx.request
  const proj = await createProjectApi(ctx.request);
  return { admin, owner, ctx, page, proj, csp };
}

// Dialog punya tombol X (aria-label Tutup) di header dan tombol Tutup di footer; pakai yang di footer.
const closeBtn = (dialog: Locator) => dialog.getByRole('button', { name: 'Tutup', exact: true }).last();

const tree = (page: Page) => page.getByRole('tree', { name: 'Pohon file knowledge' });
// Nama aksesibel treeitem ikut memuat jumlah file dan tombol menu, jadi cocokkan lewat teks nama.
const item = (page: Page, name: string) =>
  tree(page).getByRole('treeitem').filter({ has: page.getByText(name, { exact: true }) });

async function uploadFolderUi(page: Page, dir: string) {
  await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByTestId('pick-folder').setInputFiles(dir);
  await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
  return dialog;
}

async function saveWithKeyboard(page: Page) {
  await page.keyboard.press('ControlOrMeta+s');
}

test.describe('project knowledge', () => {
  test('T05-AC1 upload folder, laporan, dan pohon bertahap', async ({ browser }) => {
    const { admin, ctx, page, proj, csp } = await setup(browser);
    const { docs } = stageDocs();
    await openKnowledge(page, proj.id);
    await expect(page.getByText('Belum ada knowledge di project ini')).toBeVisible();

    await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByTestId('pick-folder').setInputFiles(docs);
    await expect(dialog.getByText(/file dilewati/)).toBeVisible();
    await expect(dialog.getByText(/1 file tidak didukung/)).toBeVisible();
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    await expect(dialog.getByRole('radio', { name: 'Gagal' })).toBeVisible();
    await closeBtn(dialog).click();

    await item(page, 'docs').click();
    await expect(item(page, 'auth')).toBeVisible();
    await item(page, 'auth').click();
    await expect(item(page, 'flow')).toBeVisible();
    await item(page, 'flow').click();
    await expect(item(page, 'x.md')).toBeVisible();
    expect(csp).toEqual([]);

    await ctx.close();
    await admin.close();
  });

  test('upload ulang: tidak berubah, lalu satu ditimpa', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    const { docs } = stageDocs();
    await openKnowledge(page, proj.id);
    let dialog = await uploadFolderUi(page, docs);
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    await closeBtn(dialog).click();

    dialog = await uploadFolderUi(page, docs);
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    await dialog.getByRole('radio', { name: 'Dibuat' }).click();
    await expect(dialog.getByRole('row')).toHaveCount(1); // hanya baris header
    await closeBtn(dialog).click();

    const changed = await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# Catatan A\n\nIsi baru.\n' }]);
    expect(changed.summary.updated).toBe(1);
    const f = await getFileApi(ctx.request, proj.id, 'docs/a.md');
    expect(f.body?.content).toContain('Isi baru.');

    await ctx.close();
    await admin.close();
  });

  test('viewer menampilkan diagram Mermaid, breadcrumb, dan salin path', async ({ browser }) => {
    const { admin, ctx, page, proj, csp } = await setup(browser);
    const { docs } = stageDocs();
    await openKnowledge(page, proj.id);
    const dialog = await uploadFolderUi(page, docs);
    await closeBtn(dialog).click();

    await item(page, 'docs').click();
    await item(page, 'auth').click();
    await item(page, 'login.md').click();
    await expect(page).toHaveURL(/\?path=docs%2Fauth%2Flogin\.md/);
    await expect(page.getByRole('navigation', { name: 'Lokasi file' })).toContainText('docs');
    await expect(page.locator('.md-body [role="img"] svg')).toBeVisible({ timeout: 20_000 });
    await page.getByRole('button', { name: 'Salin path' }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('docs/auth/login.md');
    expect(csp).toEqual([]);

    await ctx.close();
    await admin.close();
  });

  test('edit dengan Ctrl+S, tersimpan, dan bertahan setelah reload', async ({ browser }) => {
    const { admin, ctx, page, proj, csp } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# Awal\n' }]);
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md&mode=edit');

    const editor = page.getByRole('textbox', { name: 'Isi Markdown' });
    await expect(editor).toBeVisible();
    await editor.click();
    await page.keyboard.press('ControlOrMeta+End');
    await page.keyboard.type('\nBaris tambahan');
    await expect(page.getByText('Belum disimpan', { exact: true })).toBeVisible();
    await saveWithKeyboard(page);
    await expect(page.getByText('Tersimpan', { exact: true })).toBeVisible();

    await page.reload();
    await expect(page.locator('.md-body, textarea, .cm-content').first()).toContainText('Baris tambahan');
    await page.getByRole('textbox', { name: 'Isi Markdown' }).click();
    await page.keyboard.type(' lagi');
    await saveWithKeyboard(page);
    await expect(page.getByText('Tersimpan', { exact: true })).toBeVisible();
    const f = await getFileApi(ctx.request, proj.id, 'docs/a.md');
    expect(f.body?.content).toContain('lagi');
    expect(csp).toEqual([]);

    await ctx.close();
    await admin.close();
  });

  test('file baru, pindah, hapus file, hapus folder', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }]);
    await openKnowledge(page, proj.id);

    await page.getByRole('button', { name: 'File baru' }).first().click();
    await page.getByLabel('Path file').fill('notes/todo.md');
    await page.getByRole('textbox', { name: 'Isi Markdown' }).fill('# Todo');
    await page.getByRole('button', { name: 'Simpan' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Todo' })).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'notes/todo.md')).status).toBe(200);

    await item(page, 'notes').click();
    await page.getByRole('button', { name: 'Aksi untuk todo.md' }).click();
    await page.getByRole('menuitem', { name: 'Pindah atau ganti nama' }).click();
    await page.getByLabel('Path baru').fill('archive/todo.md');
    await page.getByRole('button', { name: 'Pindahkan' }).click();
    await expect(item(page, 'archive')).toBeVisible();
    await expect(item(page, 'notes')).toHaveCount(0);
    expect((await getFileApi(ctx.request, proj.id, 'archive/todo.md')).status).toBe(200);

    await page.getByRole('button', { name: 'Aksi untuk archive' }).click();
    await page.getByRole('menuitem', { name: 'Hapus folder' }).click();
    const dlg = page.getByRole('dialog');
    await expect(dlg).toContainText('1 file');
    await dlg.getByLabel('Saya mengerti semua file ini akan dihapus').check();
    await dlg.getByRole('button', { name: 'Hapus folder' }).click();
    await expect(item(page, 'archive')).toHaveCount(0);
    await expect(item(page, 'docs')).toBeVisible();

    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await page.getByRole('button', { name: 'Hapus', exact: true }).first().click();
    await page.getByRole('dialog').getByRole('button', { name: 'Hapus', exact: true }).click();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).status).toBe(404);
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await expect(page.getByText('File tidak ditemukan')).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('T05-AC4 pencarian mengurutkan, menyorot, dan membatasi folder', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    const { docs } = stageDocs();
    await openKnowledge(page, proj.id);
    const dialog = await uploadFolderUi(page, docs);
    await closeBtn(dialog).click();

    const box = page.getByRole('searchbox', { name: 'Cari knowledge' });
    await box.fill('login');
    // Dialog upload yang sudah ditutup tetap ada di DOM (memuat laporan), jadi batasi ke panel kiri.
    const side = page.getByRole('complementary', { name: 'File' });
    const results = side.getByRole('list').filter({ hasText: 'docs/auth/login.md' }).first();
    await expect(results).toBeVisible();
    await expect(results.locator('mark').first()).toBeVisible();
    const text = await results.innerText();
    expect(text.indexOf('docs/auth/login.md')).toBeLessThan(text.indexOf('docs/guide.md'));

    await box.fill('"reset password"');
    await expect(side.getByText('docs/auth/login.md')).toBeVisible();
    await expect(side.getByText('docs/guide.md')).toHaveCount(0);

    await box.fill('');
    await expect(tree(page)).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('T05-AC5 XSS: skrip, tautan javascript:, dan gambar tidak aktif', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    const { docs } = stageDocs();
    const xss = (await import('node:fs')).readFileSync(`${docs}/xss.md`, 'utf8');
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/xss.md', content: xss }]);
    let dialogFired = false;
    page.on('dialog', async (d) => {
      dialogFired = true;
      await d.dismiss();
    });
    await openKnowledge(page, proj.id, 'path=docs%2Fxss.md');
    const body = page.locator('.md-body');
    await expect(body).toContainText('Uji XSS');
    await expect(body.locator('script')).toHaveCount(0);
    await expect(body.locator('img')).toHaveCount(0);
    await expect(body.getByText('x', { exact: true })).not.toHaveAttribute('href', /.+/);
    await page.waitForTimeout(500);
    expect(dialogFired).toBe(false);

    await ctx.close();
    await admin.close();
  });

  test('T05-AC3 konflik edit bersamaan menampilkan dialog dan tidak menimpa', async ({ browser }) => {
    const { admin, ctx, page, proj, owner } = await setup(browser);
    const second = await createReadyUserApi(admin.request, { name: 'Anggota Kedua' });
    await addMemberApi(ctx.request, proj.id, second.id);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# Dasar\n' }]);

    const ctxB = await newContext(browser);
    const pageB = await ctxB.newPage();
    await loginUi(pageB, second.email, STRONG_PASSWORD);
    await expect(pageB).not.toHaveURL(/\/login/);

    await openKnowledge(page, proj.id, 'path=docs%2Fa.md&mode=edit');
    await openKnowledge(pageB, proj.id, 'path=docs%2Fa.md&mode=edit');
    const edA = page.getByRole('textbox', { name: 'Isi Markdown' });
    const edB = pageB.getByRole('textbox', { name: 'Isi Markdown' });
    await expect(edA).toBeVisible();
    await expect(edB).toBeVisible();

    await edA.click();
    await page.keyboard.type(' dari A');
    await saveWithKeyboard(page);
    await expect(page.getByText('Tersimpan', { exact: true })).toBeVisible();

    await edB.click();
    await pageB.keyboard.type(' dari B');
    await saveWithKeyboard(pageB);
    const dlg = pageB.getByRole('dialog');
    await expect(dlg).toContainText(owner.name);
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).body?.content).toContain('dari A');
    await dlg.getByRole('button', { name: 'Bandingkan' }).click();
    await expect(dlg.getByRole('heading', { name: 'Versi saya' })).toBeVisible();
    await dlg.getByRole('button', { name: 'Pakai versi terbaru' }).click();
    await expect(edB).toContainText('dari A');

    await ctxB.close();
    await ctx.close();
    await admin.close();
  });

  test('T05-AC6 non-anggota tidak melihat project (404)', async ({ browser }) => {
    const { admin, ctx, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }]);
    const outsider = await createReadyUserApi(admin.request, { name: 'Bukan Anggota' });
    const octx = await newContext(browser);
    const opage = await octx.newPage();
    await loginUi(opage, outsider.email, STRONG_PASSWORD);
    await expect(opage).not.toHaveURL(/\/login/);

    await openKnowledge(opage, proj.id, 'path=docs%2Fa.md');
    await expect(opage.getByText('Project tidak ditemukan')).toBeVisible();

    const base = `/api/projects/${proj.id}/knowledge`;
    const r = opage.request;
    expect((await r.get(`${base}/tree`, { params: { prefix: '' } })).status()).toBe(404);
    expect((await r.get(`${base}/files`, { params: { path: 'docs/a.md' } })).status()).toBe(404);
    expect((await r.get(`${base}/search`, { params: { q: 'a' } })).status()).toBe(404);
    expect((await r.post(`${base}/files`, { headers: CSRF, data: { path: 'x.md', content: 'x' } })).status()).toBe(404);
    expect((await r.put(`${base}/files`, { headers: CSRF, params: { path: 'docs/a.md' }, data: { content: 'x', version: 'a'.repeat(64) } })).status()).toBe(404);
    expect((await r.delete(`${base}/files`, { headers: CSRF, params: { path: 'docs/a.md' } })).status()).toBe(404);
    expect((await r.delete(`${base}/folders`, { headers: CSRF, params: { path: 'docs' } })).status()).toBe(404);
    expect((await r.post(`${base}/files/move`, { headers: CSRF, data: { from: 'docs/a.md', to: 'b.md' } })).status()).toBe(404);
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).status).toBe(200);

    await octx.close();
    await ctx.close();
    await admin.close();
  });

  test('CSRF: POST tanpa header X-Requested-With ditolak 403', async ({ browser }) => {
    const { admin, ctx, proj } = await setup(browser);
    const res = await ctx.request.post(`/api/projects/${proj.id}/knowledge/files`, { data: { path: 'x.md', content: 'x' } });
    expect(res.status()).toBe(403);
    expect((await res.json()).error.code).toBe('CSRF_REQUIRED');
    await ctx.close();
    await admin.close();
  });

  test('path tidak valid dan path sudah ada ditolak', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }]);
    await openKnowledge(page, proj.id, 'new=1');
    const path = page.getByLabel('Path file');
    await path.fill('../x.md');
    await expect(page.getByText('Path tidak boleh memuat segmen . atau ..')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Simpan' })).toBeDisabled();
    await path.fill('.env');
    await expect(page.getByText('File atau folder tersembunyi (diawali titik) tidak didukung')).toBeVisible();
    await path.fill('config.yaml');
    await expect(page.getByText('Hanya file .md, .markdown, dan .txt yang didukung')).toBeVisible();

    const bad = await ctx.request.post(`/api/projects/${proj.id}/knowledge/files`, { headers: CSRF, data: { path: '../x.md', content: 'x' } });
    expect(bad.status()).toBe(400);
    expect((await bad.json()).error.details.reason).toBe('path_traversal');

    await path.fill('docs/a.md');
    await page.getByRole('textbox', { name: 'Isi Markdown' }).fill('# Timpa');
    await page.getByRole('button', { name: 'Simpan' }).click();
    await expect(page.getByText('File dengan path ini sudah ada')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Buka file yang ada' })).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).body?.content).toBe('# A\n');

    await ctx.close();
    await admin.close();
  });

  test('T05-AC7 audit mencatat knowledge.upload.completed', async ({ browser }) => {
    const { admin, ctx, proj } = await setup(browser);
    const files = Array.from({ length: 100 }, (_, i) => ({ path: `bulk/f${i}.md`, content: `# F${i}\n` }));
    const r = await uploadApi(ctx.request, proj.id, files);
    expect(r.summary.created).toBe(100);
    const res = await admin.request.get('/api/audit-log', { params: { action: 'knowledge.upload.completed' } });
    expect(res.status()).toBe(200);
    expect(JSON.stringify(await res.json())).toContain('knowledge.upload.completed');
    await ctx.close();
    await admin.close();
  });

  test('upload ke folder lewat menu folder menyimpan di bawah folder itu', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }]);
    await openKnowledge(page, proj.id);

    await page.getByRole('button', { name: 'Aksi untuk docs' }).click();
    await page.getByRole('menuitem', { name: 'Upload ke sini' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toContainText('Tujuan: docs');
    await dialog.getByTestId('pick-files').setInputFiles({ name: 'extra.md', mimeType: 'text/markdown', buffer: Buffer.from('# Extra\n') });
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    await closeBtn(dialog).click();

    expect((await getFileApi(ctx.request, proj.id, 'docs/extra.md')).status).toBe(200);
    await item(page, 'docs').click();
    await expect(item(page, 'extra.md')).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('drag-drop dua file .md menyimpan keduanya di root', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await openKnowledge(page, proj.id);
    await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
    const dialog = page.getByRole('dialog');

    const dataTransfer = await page.evaluateHandle(() => {
      const dt = new DataTransfer();
      dt.items.add(new File(['# Satu\n'], 'satu.md', { type: 'text/markdown' }));
      dt.items.add(new File(['# Dua\n'], 'dua.md', { type: 'text/markdown' }));
      return dt;
    });
    await dialog.getByTestId('dropzone').dispatchEvent('drop', { dataTransfer });
    await expect(dialog.getByText('2 file ke Knowledge')).toBeVisible();
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    await closeBtn(dialog).click();

    expect((await getFileApi(ctx.request, proj.id, 'satu.md')).status).toBe(200);
    expect((await getFileApi(ctx.request, proj.id, 'dua.md')).status).toBe(200);
    await expect(item(page, 'satu.md')).toBeVisible();
    await expect(item(page, 'dua.md')).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('Super Admin non-anggota bisa melihat, mengedit, dan upload', async ({ browser }) => {
    const { admin, ctx, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# Awal\n' }]);
    const page = await admin.newPage();
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await expect(page.getByRole('heading', { level: 1, name: 'Awal' })).toBeVisible();

    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    const editor = page.getByRole('textbox', { name: 'Isi Markdown' });
    await editor.click();
    await page.keyboard.press('ControlOrMeta+End');
    await page.keyboard.type('\nDari admin');
    await saveWithKeyboard(page);
    await expect(page.getByText('Tersimpan', { exact: true })).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).body?.content).toContain('Dari admin');

    await openKnowledge(page, proj.id);
    await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByTestId('pick-files').setInputFiles({ name: 'admin.md', mimeType: 'text/markdown', buffer: Buffer.from('# Admin\n') });
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByText('Laporan upload')).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'admin.md')).status).toBe(200);

    await ctx.close();
    await admin.close();
  });

  test('T05-AC2 upload lewat UI: file 2 MB, biner, .git, dan png dilaporkan; dua file valid masuk', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    const root = mkdtempSync(join(tmpdir(), 'knowledge-ac2-'));
    const dir = join(root, 'bundle');
    mkdirSync(join(dir, '.git'), { recursive: true });
    writeFileSync(join(dir, 'ok1.md'), '# Satu\n');
    writeFileSync(join(dir, 'ok2.md'), '# Dua\n');
    writeFileSync(join(dir, 'besar.md'), '# Besar\n' + 'x'.repeat(2 * 1024 * 1024));
    writeFileSync(join(dir, 'biner.md'), Buffer.from([0xff, 0xfe, 0x00, 0x80, 0x00, 0xc3, 0x28]));
    writeFileSync(join(dir, 'logo.png'), Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]));
    writeFileSync(join(dir, '.git', 'config'), '[core]\n');

    await openKnowledge(page, proj.id);
    await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByTestId('pick-folder').setInputFiles(dir);
    await expect(dialog.getByText('1 file dilewati (tersembunyi)')).toBeVisible();
    await expect(dialog.getByText('1 file tidak didukung')).toBeVisible();
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByText('Laporan upload')).toBeVisible();

    const rowOf = (name: string) => dialog.getByRole('row').filter({ hasText: name });
    await expect(rowOf('bundle/besar.md')).toContainText('Ditolak');
    await expect(rowOf('bundle/besar.md')).toContainText('melebihi batas');
    await expect(rowOf('bundle/biner.md')).toContainText('Ditolak');
    await expect(rowOf('bundle/biner.md')).toContainText('UTF-8');
    await expect(rowOf('bundle/.git/config')).toContainText('Dilewati');
    await expect(rowOf('bundle/logo.png')).toContainText('Tidak didukung');
    await expect(rowOf('bundle/ok1.md')).toContainText('Dibuat');
    await expect(rowOf('bundle/ok2.md')).toContainText('Dibuat');

    await dialog.getByRole('radio', { name: 'Gagal' }).click();
    await expect(dialog.getByRole('row')).toHaveCount(1 + 3); // header + besar, biner, logo
    await closeBtn(dialog).click();

    await item(page, 'bundle').click();
    await expect(item(page, 'ok1.md')).toBeVisible();
    await expect(item(page, 'ok2.md')).toBeVisible();
    await expect(item(page, 'besar.md')).toHaveCount(0);
    expect((await getFileApi(ctx.request, proj.id, 'bundle/.git/config')).status).toBe(404);

    await ctx.close();
    await admin.close();
  });

  test('pindah ke path yang sudah ada ditolak dan kedua file utuh', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [
      { path: 'docs/a.md', content: '# A\n' },
      { path: 'docs/b.md', content: '# B\n' },
    ]);
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await page.getByRole('button', { name: 'Pindah atau ganti nama' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Path baru').fill('docs/b.md');
    await dialog.getByRole('button', { name: 'Pindahkan' }).click();
    await expect(dialog.getByText('File dengan path ini sudah ada')).toBeVisible();
    await expect(dialog).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).body?.content).toBe('# A\n');
    expect((await getFileApi(ctx.request, proj.id, 'docs/b.md')).body?.content).toBe('# B\n');

    await ctx.close();
    await admin.close();
  });

  test('hapus file dua kali di dua tab: tab kedua menampilkan "sudah tidak ada" tanpa crash', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }, { path: 'docs/b.md', content: '# B\n' }]);
    const page2 = await ctx.newPage();
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await openKnowledge(page2, proj.id, 'path=docs%2Fa.md');
    await expect(page.getByRole('heading', { level: 1, name: 'A', exact: true })).toBeVisible();
    await expect(page2.getByRole('heading', { level: 1, name: 'A', exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Hapus', exact: true }).first().click();
    await page.getByRole('dialog').getByRole('button', { name: 'Hapus', exact: true }).click();
    await expect.poll(async () => (await getFileApi(ctx.request, proj.id, 'docs/a.md')).status).toBe(404);

    await page2.getByRole('button', { name: 'Hapus', exact: true }).first().click();
    await page2.getByRole('dialog').getByRole('button', { name: 'Hapus', exact: true }).click();
    await expect(page2.getByText('File sudah tidak ada')).toBeVisible();
    await expect(page2.getByRole('tree', { name: 'Pohon file knowledge' })).toBeVisible();
    await expect(page2.getByTitle('docs/a.md')).toHaveCount(0);

    await ctx.close();
    await admin.close();
  });

  test('folder dihapus di tab lain lalu dibuka di pohon menampilkan "Folder kosong"', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [
      { path: 'docs/a.md', content: '# A\n' },
      { path: 'docs/sub/x.md', content: '# X\n' },
    ]);
    await openKnowledge(page, proj.id);
    await item(page, 'docs').click();
    await expect(item(page, 'sub')).toBeVisible();

    const del = await ctx.request.delete(`/api/projects/${proj.id}/knowledge/folders`, { headers: CSRF, params: { path: 'docs/sub' } });
    expect(del.status()).toBe(200);

    await item(page, 'sub').click();
    await expect(page.getByText('Folder kosong')).toBeVisible();
    await expect(page.getByRole('alert')).toHaveCount(0);

    await ctx.close();
    await admin.close();
  });

  test('pencarian dengan karakter aneh tidak memunculkan error', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    const { docs } = stageDocs();
    await openKnowledge(page, proj.id);
    const dialog = await uploadFolderUi(page, docs);
    await closeBtn(dialog).click();

    const box = page.getByRole('searchbox', { name: 'Cari knowledge' });
    for (const q of ['" & : or -', '"unclosed', 'a'.repeat(300)]) {
      await box.fill(q);
      await expect(page.getByText('Tidak ada hasil').or(page.locator('mark').first())).toBeVisible();
      await expect(page.getByRole('alert')).toHaveCount(0);
    }

    await ctx.close();
    await admin.close();
  });

  test('sesi kedaluwarsa saat edit: login ulang lalu pulihkan draf dan simpan', async ({ browser }) => {
    const { admin, ctx, page, proj, owner } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# Awal\n' }]);
    await openKnowledge(page, proj.id, 'path=docs%2Fa.md&mode=edit');
    const editor = page.getByRole('textbox', { name: 'Isi Markdown' });
    await editor.click();
    await page.keyboard.press('ControlOrMeta+End');
    await page.keyboard.type('\nDraf penting');
    await expect(page.getByText('Belum disimpan', { exact: true })).toBeVisible();
    await page.waitForTimeout(1300); // draf disimpan dengan debounce 1 detik

    await ctx.clearCookies();
    await saveWithKeyboard(page);
    await expect(page).toHaveURL(/\/login\?next=.*mode%3Dedit/);

    await submitLogin(page, owner.email, STRONG_PASSWORD);
    await expect(page).toHaveURL(/mode=edit/);
    await expect(page.getByText(/Ada draf belum disimpan/)).toBeVisible();
    await page.getByRole('button', { name: 'Pulihkan' }).click();
    await expect(page.getByRole('textbox', { name: 'Isi Markdown' })).toContainText('Draf penting');
    await page.getByRole('textbox', { name: 'Isi Markdown' }).click(); // fokus ke editor agar Ctrl+S sampai ke handler
    await saveWithKeyboard(page);
    await expect(page.getByText('Tersimpan', { exact: true })).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).body?.content).toContain('Draf penting');

    await ctx.close();
    await admin.close();
  });

  test('Batal pada dialog hapus file dan folder tidak mengirim DELETE', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, [{ path: 'docs/a.md', content: '# A\n' }]);
    const deletes: string[] = [];
    page.on('request', (r) => {
      if (r.method() === 'DELETE') deletes.push(r.url());
    });

    await openKnowledge(page, proj.id, 'path=docs%2Fa.md');
    await page.getByRole('button', { name: 'Hapus', exact: true }).first().click();
    let dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Batal' }).click();
    await expect(dialog).toBeHidden();

    await page.getByRole('button', { name: 'Aksi untuk docs' }).click();
    await page.getByRole('menuitem', { name: 'Hapus folder' }).click();
    dialog = page.getByRole('dialog');
    await expect(dialog).toContainText('1 file');
    await dialog.getByRole('button', { name: 'Batal' }).click();
    await expect(dialog).toBeHidden();

    await page.waitForTimeout(500);
    expect(deletes).toEqual([]);
    await expect(item(page, 'docs')).toBeVisible();
    expect((await getFileApi(ctx.request, proj.id, 'docs/a.md')).status).toBe(200);

    await ctx.close();
    await admin.close();
  });
});
