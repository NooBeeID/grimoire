import { expect, test, type Browser, type BrowserContext } from '@playwright/test';
import { createKeyApi, revokeKeyApi, uniqueKeyName } from './helpers/apikeys';
import { adminContext } from './helpers/auth';
import { bigText, getFileApi, uploadApi } from './helpers/knowledge';
import {
  ListStructured,
  SearchStructured,
  callTool,
  nextOffset,
  stripMarker,
  textOf,
} from './helpers/knowledge-tools';
import { expectRpcFailure, expectRpcResult, mcpPost, toolsListBody, ToolsListResult } from './helpers/mcp';
import { createProjectApi, uniqueProjectName } from './helpers/projects';

// E2E T08 discovery/knowledge-tools terhadap binary sungguhan (butuh T01-T05 + T07 + T08).
// Panggilan MCP memakai fixture `request` tanpa cookie; jejak diverifikasi lewat layar /audit-log (admin).

const BIG_BYTES = 200 * 1024;
const SECRET_B = 'SECRET-B-7f3a';

interface Env {
  admin: BrowserContext;
  projectA: { id: string; name: string };
  projectB: { id: string; name: string };
  keyId: string;
  keyName: string;
  prefix: string;
  rawKey: string;
}

async function setup(browser: Browser): Promise<Env> {
  const admin = await adminContext(browser, 'id-ID');
  const projectA = await createProjectApi(admin.request, uniqueProjectName('Payments API'));
  const projectB = await createProjectApi(admin.request, uniqueProjectName('Billing Internal'));
  const keyName = uniqueKeyName('claude-laptop');
  const created = await createKeyApi(admin.request, projectA.id, keyName);
  await uploadApi(admin.request, projectA.id, [
    { path: 'readme.md', content: '# Payments API knowledge\nMulai dari docs/.' },
    { path: 'docs/auth/login-flow.md', content: '# Alur login\nServer menerbitkan session cookie setelah verifikasi.' },
    { path: 'docs/auth/readme.md', content: '# Auth\nRingkasan folder auth.' },
    { path: 'docs/guides/rate-limit.md', content: 'Rate limiting per key: 60 request per menit.' },
    { path: 'docs/big.md', content: bigText(BIG_BYTES) },
  ]);
  await uploadApi(admin.request, projectB.id, [{ path: 'secret/b.md', content: SECRET_B }]);
  return { admin, projectA, projectB, keyId: created.key.id, keyName, prefix: created.key.prefix, rawKey: created.raw_key };
}

test.describe('helper: penanda', () => {
  test('nextOffset', () => {
    expect(nextOffset('isi\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]')).toBe(49998);
    expect(nextOffset('isi\n\n[ada 3 file lagi, panggil list_files dengan offset=2 recursive=true]')).toBe(2);
    expect(nextOffset('tanpa penanda')).toBeNull();
    expect(nextOffset('baris offset=5 di isi\nberikutnya')).toBeNull();
  });
  test('stripMarker', () => {
    expect(stripMarker('isi\n\n[terpotong: lanjutkan dengan offset=9 dari total 20 byte]')).toBe('isi');
    expect(stripMarker('isi biasa\n\ndua paragraf')).toBe('isi biasa\n\ndua paragraf');
  });
});

test.describe('knowledge-tools', () => {
  test.describe.configure({ mode: 'serial' });
  let env: Env;

  test.beforeAll(async ({ browser }) => {
    env = await setup(browser);
  });
  test.afterAll(async () => {
    await env?.admin.close();
  });

  test('P1 tools/list memuat tiga tool read-only tanpa project_id', async ({ request }) => {
    const { tools } = ToolsListResult.parse(await expectRpcResult(await mcpPost(request, env.rawKey, toolsListBody(1))));
    for (const name of ['list_files', 'read_file', 'search_knowledge']) {
      const t = tools.find((x) => x.name === name);
      expect(t, name).toBeDefined();
      expect(t!.annotations.readOnlyHint).toBe(true);
      expect(t!.inputSchema.type).toBe('object');
      expect(t!.inputSchema.additionalProperties).toBe(false);
      expect(Object.keys((t!.inputSchema.properties as Record<string, unknown>) ?? {})).not.toContain('project_id');
    }
  });

  test('P2 list_files akar menampilkan satu tingkat', async ({ request }) => {
    const r = await callTool(request, env.rawKey, 'list_files', {});
    expect(r.isError).toBe(false);
    expect(textOf(r)).toContain('readme.md');
    expect(textOf(r)).toContain('[folder] docs/');
    expect(textOf(r)).not.toContain('docs/auth/login-flow.md');
    const s = ListStructured.parse(r.structuredContent);
    expect(s.folders).toEqual(['docs']);
    expect(s.next_offset).toBeNull();
  });

  test('P3 list_files turun tingkat demi tingkat', async ({ request }) => {
    const docs = await callTool(request, env.rawKey, 'list_files', { path: 'docs' });
    expect(textOf(docs)).toContain('[folder] docs/auth/');
    expect(textOf(docs)).toContain('[folder] docs/guides/');
    expect(textOf(docs)).toContain('docs/big.md');
    const auth = await callTool(request, env.rawKey, 'list_files', { path: 'docs/auth' });
    expect(textOf(auth)).toContain('docs/auth/login-flow.md');
    expect(textOf(auth)).toContain('docs/auth/readme.md');
  });

  test('P4 list_files rekursif terpaginasi mengikuti penanda', async ({ request }) => {
    const paths: string[] = [];
    let offset = 0;
    for (let guard = 0; guard < 10; guard++) {
      const r = await callTool(request, env.rawKey, 'list_files', { recursive: true, limit: 2, offset });
      const s = ListStructured.parse(r.structuredContent);
      expect(s.total).toBe(5);
      paths.push(...s.items.map((i) => i.path));
      const next = nextOffset(textOf(r));
      if (next === null) break;
      expect(next).toBe(offset + 2);
      offset = next;
    }
    expect(new Set(paths).size).toBe(5);
    expect(paths).toHaveLength(5);
  });

  test('P5 read_file berkas besar dibaca utuh lewat potongan', async ({ request }) => {
    let offset = 0;
    let joined = '';
    let chunks = 0;
    for (let guard = 0; guard < 50; guard++) {
      const r = await callTool(request, env.rawKey, 'read_file', { path: 'docs/big.md', limit: 50000, offset });
      expect(r.isError).toBe(false);
      expect(r._meta?.truncated).toBeUndefined();
      joined += stripMarker(textOf(r));
      chunks++;
      const next = nextOffset(textOf(r));
      if (next === null) break;
      offset = next;
    }
    expect(chunks).toBeGreaterThanOrEqual(4);
    expect(joined).toBe(bigText(BIG_BYTES));
  });

  test('P6 read_file sama dengan isi yang dilihat manusia', async ({ request }) => {
    const path = 'docs/auth/login-flow.md';
    const r = await callTool(request, env.rawKey, 'read_file', { path });
    const human = await getFileApi(env.admin.request, env.projectA.id, path);
    expect(r.isError).toBe(false);
    expect(textOf(r)).toBe(human.body!.content);
    expect(nextOffset(textOf(r))).toBeNull();
  });

  test('P7-P9 search_knowledge', async ({ request }) => {
    const hit = await callTool(request, env.rawKey, 'search_knowledge', { query: 'rate limit' });
    expect(hit.isError).toBe(false);
    expect(textOf(hit)).toContain('docs/guides/rate-limit.md');
    expect(textOf(hit)).toMatch(/\*\*[Rr]ate\*\*/);
    const s = SearchStructured.parse(hit.structuredContent);
    expect(s.total).toBeGreaterThanOrEqual(1);
    expect(s.items[0].path).toBe('docs/guides/rate-limit.md');
    expect((await callTool(request, env.rawKey, 'read_file', { path: s.items[0].path })).isError).toBe(false);

    const scoped = await callTool(request, env.rawKey, 'search_knowledge', { query: 'rate', path: 'docs/auth' });
    for (const i of SearchStructured.parse(scoped.structuredContent).items) expect(i.path.startsWith('docs/auth/')).toBe(true);

    const none = await callTool(request, env.rawKey, 'search_knowledge', { query: 'zzqx-tidak-ada' });
    expect(none.isError).toBe(false);
    expect(textOf(none)).toContain('Tidak ada hasil');
    expect(textOf(none)).toContain('list_files');
    expect(SearchStructured.parse(none.structuredContent).total).toBe(0);
  });

  test('P10 project kosong', async ({ request }) => {
    const c = await createProjectApi(env.admin.request, uniqueProjectName('Onboarding Docs'));
    const key = await createKeyApi(env.admin.request, c.id, uniqueKeyName('kosong'));
    const r = await callTool(request, key.raw_key, 'list_files', {});
    expect(r.isError).toBe(false);
    expect(textOf(r)).toContain('Belum ada file knowledge di project ini.');
  });

  test('N1-N4, N6-N9 argumen tidak valid dan project_id ditolak', async ({ request }) => {
    const cases: [string, unknown, string[]][] = [
      ['read_file', {}, ['Argumen tidak valid: path wajib diisi']],
      ['list_files', { limit: 900 }, ['limit harus 1-500 (diterima: 900)']],
      ['read_file', { path: 'secret/b.md', project_id: env.projectB.id }, ['ditentukan oleh API key']],
      ['list_files', { project_id: env.projectB.id }, ['ditentukan oleh API key']],
      ['search_knowledge', { query: '   ' }, ['query wajib diisi 1-200 karakter']],
      ['search_knowledge', { query: 'x'.repeat(201) }, ['query wajib diisi 1-200 karakter']],
      ['read_file', { path: 'docs/auth/login-flow.md', offset: 999999999 }, ['Permintaan ditolak:']],
      ['list_files', { path: '../etc' }, ['tidak boleh memuat segmen ..']],
      ['list_files', { recursive: 'true' }, ['recursive harus boolean']],
    ];
    for (const [name, args, phrases] of cases) {
      const r = await callTool(request, env.rawKey, name, args);
      expect(r.isError, `${name} ${JSON.stringify(args)}`).toBe(true);
      for (const p of phrases) expect(textOf(r)).toContain(p);
      expect(textOf(r)).not.toContain(SECRET_B);
    }
  });

  test('N3 tanpa project_id, path project B tidak ditemukan di project A', async ({ request }) => {
    const r = await callTool(request, env.rawKey, 'read_file', { path: 'secret/b.md' });
    expect(r.isError).toBe(true);
    expect(textOf(r)).toContain('File tidak ditemukan');
    expect(textOf(r)).not.toContain(SECRET_B);
  });

  test('N5 salah ketik memberi saran path', async ({ request }) => {
    const r = await callTool(request, env.rawKey, 'read_file', { path: 'docs/auth/login-flw.md' });
    expect(r.isError).toBe(true);
    for (const p of ['File tidak ditemukan', 'Similar:', '- docs/auth/login-flow.md', 'list_files']) expect(textOf(r)).toContain(p);
  });

  test('N8 karakter khusus pencarian tidak memicu error JSON-RPC', async ({ request }) => {
    for (const query of ['a & b | !c:* (x)', `'"`, '%_\\']) {
      const r = await callTool(request, env.rawKey, 'search_knowledge', { query });
      expect(r.isError, query).toBe(false);
    }
  });

  test('N10 key yang dicabut ditolak gateway', async ({ request }) => {
    const created = await createKeyApi(env.admin.request, env.projectA.id, uniqueKeyName('dicabut'));
    await revokeKeyApi(env.admin.request, env.projectA.id, created.key.id);
    await expectRpcFailure(
      await mcpPost(request, created.raw_key, { jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'list_files', arguments: {} } }),
      401,
      -32001,
    );
  });

  test('P11 jejak agent tampil di /audit-log tanpa isi file', async () => {
    const page = await env.admin.newPage();
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.keyId}`);
    await expect(page.getByRole('table')).toBeVisible();
    for (const action of ['mcp.list_files', 'mcp.read_file', 'mcp.search_knowledge']) {
      await expect(page.getByRole('row').filter({ hasText: action }).first()).toContainText(`${env.keyName} (${env.prefix})`);
    }

    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.keyId}&action=mcp.read_file`);
    const okRow = page.getByRole('row').filter({ hasText: 'docs/auth/login-flow.md' }).first();
    await expect(okRow).toContainText('Berhasil');
    await okRow.click();
    const detail = page.getByText('total_bytes').first();
    await expect(detail).toBeVisible();
    await expect(page.getByText('session cookie')).toHaveCount(0);
    await expect(page.getByText(SECRET_B)).toHaveCount(0);
  });

  test('N11 kegagalan tool tercatat Gagal dengan error_code', async () => {
    const page = await env.admin.newPage();
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.keyId}&action=mcp.read_file`);
    const failRow = page.getByRole('row').filter({ hasText: 'Gagal' }).first();
    await expect(failRow).toBeVisible();
    await failRow.click();
    await expect(page.getByText('not_found').first()).toBeVisible();
  });
});
