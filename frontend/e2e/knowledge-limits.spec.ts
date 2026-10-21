import { expect, test, type Browser } from '@playwright/test';
import { adminContext as adminContextIn, createReadyUserApi, loginUi, newContext as newContextIn, STRONG_PASSWORD } from './helpers/auth';
import { openKnowledge, uploadApi } from './helpers/knowledge';
import { createProjectApi } from './helpers/projects';

// Server kedua: MAX_UPLOAD_BYTES=2097152, MAX_PROJECT_FILES=5 (lihat playwright.config.ts).

const adminContext = (browser: Browser) => adminContextIn(browser, 'id-ID');
const newContext = (browser: Browser) => newContextIn(browser, 'id-ID');

async function setup(browser: Browser) {
  const admin = await adminContext(browser);
  const owner = await createReadyUserApi(admin.request, { name: 'Pemilik Batas' });
  const ctx = await newContext(browser);
  const page = await ctx.newPage();
  await loginUi(page, owner.email, STRONG_PASSWORD);
  await expect(page).not.toHaveURL(/\/login/); // sesi harus ada sebelum memakai ctx.request
  const proj = await createProjectApi(ctx.request);
  return { admin, ctx, page, proj };
}

const mb = (n: number, tag: string) => Buffer.from(`# ${tag}\n${'x'.repeat(n * 1024 * 1024 - 16)}\n`);

test.describe('knowledge limits', () => {
  test('T05 batas ukuran total upload menampilkan banner dan tidak mengubah pohon', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await openKnowledge(page, proj.id);
    await page.getByRole('button', { name: 'Upload', exact: true }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByTestId('pick-files').setInputFiles(
      ['a', 'b', 'c'].map((n) => ({ name: `${n}.md`, mimeType: 'text/markdown', buffer: mb(1, n) })),
    );
    await dialog.getByRole('button', { name: 'Unggah', exact: true }).click();
    await expect(dialog.getByRole('alert').first()).toContainText('Ukuran total upload melebihi batas 2 MB');
    await dialog.getByRole('button', { name: 'Tutup', exact: true }).last().click();
    await expect(page.getByText('Belum ada knowledge di project ini')).toBeVisible();

    await ctx.close();
    await admin.close();
  });

  test('T05 batas jumlah file project: baru ditolak, yang ada tetap bisa ditimpa', async ({ browser }) => {
    const { admin, ctx, page, proj } = await setup(browser);
    await uploadApi(ctx.request, proj.id, ['a', 'b', 'c', 'd'].map((n) => ({ path: `f/${n}.md`, content: `# ${n}\n` })));

    const r = await uploadApi(ctx.request, proj.id, [
      { path: 'f/a.md', content: '# a diubah\n' },
      { path: 'g/e.md', content: '# e\n' },
      { path: 'g/f.md', content: '# f\n' },
      { path: 'g/g.md', content: '# g\n' },
    ]);
    expect(r.summary.updated).toBe(1);
    expect(r.summary.created).toBe(1);
    expect(r.summary.rejected).toBe(2);
    expect(r.items.filter((i) => i.reason === 'project_file_limit')).toHaveLength(2);

    // Project kini penuh (5 file): file baru dari editor ditolak.
    await openKnowledge(page, proj.id, 'new=1');
    await page.getByLabel('Path file').fill('notes/baru.md');
    await page.getByRole('textbox', { name: 'Isi Markdown' }).fill('# Baru');
    await page.getByRole('button', { name: 'Simpan' }).click();
    await expect(page.getByText(/batas jumlah file/i)).toBeVisible();

    await ctx.close();
    await admin.close();
  });
});
