import { expect, test, type Browser, type BrowserContext } from '@playwright/test';
import { createKeyApi, revokeKeyApi, uniqueKeyName } from './helpers/apikeys';
import {
  EndpointDetail,
  billingYaml,
  hugeYaml,
  manyEndpointsYaml,
  nextOffset,
  parseDetail,
  parseEndpointLines,
  petstoreYaml,
  soloYaml,
  uploadSpecsApi,
} from './helpers/apispecs';
import { adminContext } from './helpers/auth';
import { callTool, textOf } from './helpers/knowledge-tools';
import { expectRpcFailure, expectRpcResult, mcpPost, toolsCallBody, toolsListBody, ToolsListResult } from './helpers/mcp';
import { createProjectApi, uniqueProjectName } from './helpers/projects';

// E2E T09 discovery/api-spec-tools terhadap binary sungguhan (butuh T01-T07 + T09).
// Panggilan MCP memakai fixture `request` tanpa cookie; jejak diverifikasi lewat /audit-log (admin).

const A_ENDPOINTS = 3 + 2 + 1; // petstore + billing + huge
const UNAUTHORIZED = -32001;

const DETAIL_PETSTORE = { spec: 'petstore', method: 'get', path: '/pets/{petId}' };

interface Project {
  id: string;
  name: string;
}
interface Agent {
  keyId: string;
  keyName: string;
  prefix: string;
  rawKey: string;
}
interface Env {
  admin: BrowserContext;
  A: Project;
  B: Project;
  C: Project;
  D: Project;
  kA: Agent;
  kA2: Agent;
  kB: Agent;
  kC: Agent;
  kD: Agent;
}

async function agentFor(env: Pick<Env, 'admin'>, project: Project, label: string): Promise<Agent> {
  const keyName = uniqueKeyName(label);
  const created = await createKeyApi(env.admin.request, project.id, keyName);
  return { keyId: created.key.id, keyName, prefix: created.key.prefix, rawKey: created.raw_key };
}

async function setup(browser: Browser): Promise<Env> {
  const admin = await adminContext(browser, 'id-ID');
  const A = await createProjectApi(admin.request, uniqueProjectName('Payments e2e'));
  const B = await createProjectApi(admin.request, uniqueProjectName('Ledger e2e'));
  const C = await createProjectApi(admin.request, uniqueProjectName('Catalog e2e'));
  const D = await createProjectApi(admin.request, uniqueProjectName('Onboarding e2e'));
  await uploadSpecsApi(admin.request, A.id, [
    { file: 'petstore.yaml', content: petstoreYaml },
    { file: 'billing.yaml', content: billingYaml },
    { file: 'huge.yaml', content: hugeYaml() },
  ]);
  await uploadSpecsApi(admin.request, B.id, [{ file: 'solo.yaml', content: soloYaml }]);
  await uploadSpecsApi(admin.request, C.id, [{ file: 'catalog.yaml', content: manyEndpointsYaml(120) }]);
  const base = { admin };
  return {
    admin,
    A,
    B,
    C,
    D,
    kA: await agentFor(base, A, 'claude-laptop'),
    kA2: await agentFor(base, A, 'claude-laptop-neg'),
    kB: await agentFor(base, B, 'ledger-bot'),
    kC: await agentFor(base, C, 'catalog-bot'),
    kD: await agentFor(base, D, 'onboarding-bot'),
  };
}

test.describe('helper: parser T09', () => {
  const page1 = [
    'Endpoint 1-2 dari 3 | spec: semua | tag: pets',
    '== petstore ==',
    'GET /pets — List all pets [listPets] (pets)',
    'POST /pets — Create a pet [createPet] (pets) [deprecated]',
    '',
    '[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]',
    'Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id).',
  ].join('\n');

  test('nextOffset', () => {
    expect(nextOffset(page1)).toBe(2);
    expect(nextOffset('Endpoint 1-6 dari 6 | spec: semua')).toBeNull();
    expect(nextOffset('Endpoint 1-1 dari 1 | query: "offset=5"\nGET /a — x [y]')).toBeNull();
  });

  test('parseEndpointLines', () => {
    const rows = parseEndpointLines(page1);
    expect(rows).toHaveLength(2);
    expect(rows.every((r) => r.group === 'petstore')).toBe(true);
    expect(rows[1]).toMatchObject({ method: 'POST', path: '/pets', operationId: 'createPet', tags: ['pets'], deprecated: true });
    expect(rows[0].deprecated).toBe(false);
    expect(parseEndpointLines('== x ==\nGET /bare')).toEqual([
      { group: 'x', method: 'GET', path: '/bare', operationId: '', tags: [], deprecated: false },
    ]);
  });

  test('parseDetail', () => {
    const ok = JSON.stringify({
      spec: 'petstore',
      method: 'GET',
      path: '/pets/{petId}',
      schema_depth: 4,
      truncated: false,
      operation: { operationId: 'showPetById' },
      servers: [{ url: 'https://petstore.noobee.id/v1' }],
      securitySchemes: {},
    });
    expect(parseDetail(ok).operation.operationId).toBe('showPetById');
    expect(() => parseDetail('bukan json')).toThrow(/bukan JSON valid/);
    expect(EndpointDetail.safeParse({}).success).toBe(false);
  });

  test('manyEndpointsYaml deterministik dan unik', () => {
    const y = manyEndpointsYaml(120);
    expect(y).toBe(manyEndpointsYaml(120));
    const paths = y.match(/^ {2}\/items\/\d{3}:$/gm) ?? [];
    expect(new Set(paths).size).toBe(120);
  });

  test('hugeYaml kecil di file, deterministik', () => {
    const y = hugeYaml();
    expect(y).toBe(hugeYaml());
    expect(Buffer.byteLength(y)).toBeLessThan(1024 * 1024);
    expect(Buffer.byteLength(y)).toBeLessThan(5242880);
  });
});

test.describe('api-spec-tools', () => {
  test.describe.configure({ mode: 'serial' });
  let env: Env;

  test.beforeAll(async ({ browser }) => {
    env = await setup(browser);
  });
  test.afterAll(async () => {
    await env?.admin.close();
  });

  test('P1 tools/list memuat tiga tool read-only tanpa project_id', async ({ request }) => {
    const { tools } = ToolsListResult.parse(await expectRpcResult(await mcpPost(request, env.kA.rawKey, toolsListBody(1))));
    for (const name of ['list_api_specs', 'list_api_endpoints', 'get_api_endpoint']) {
      const t = tools.find((x) => x.name === name);
      expect(t, name).toBeDefined();
      expect(t!.annotations.readOnlyHint).toBe(true);
      expect(t!.inputSchema.type).toBe('object');
      expect(t!.inputSchema.additionalProperties).toBe(false);
      const props = (t!.inputSchema.properties ?? {}) as Record<string, Record<string, unknown>>;
      for (const banned of ['project_id', 'projectId', 'project']) expect(Object.keys(props)).not.toContain(banned);
      if (name === 'list_api_endpoints') {
        expect(props.limit.maximum).toBe(300);
        expect(props.limit.default).toBe(100);
      }
      if (name === 'get_api_endpoint') {
        expect(props.schema_depth.maximum).toBe(8);
        expect(props.schema_depth.default).toBe(4);
      }
    }
  });

  test('P2 list_api_specs mengurutkan spec dan memberi langkah berikut', async ({ request }) => {
    const r = await callTool(request, env.kA.rawKey, 'list_api_specs', {});
    expect(r.isError).toBe(false);
    expect(r.structuredContent).toBeUndefined();
    const text = textOf(r);
    expect(text.startsWith('3 API spec di project ini:')).toBe(true);
    const iBilling = text.indexOf('- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint');
    const iHuge = text.indexOf('- huge: Huge Report');
    const iPet = text.indexOf('- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint');
    expect(iBilling).toBeGreaterThanOrEqual(0);
    expect(iHuge).toBeGreaterThan(iBilling);
    expect(iPet).toBeGreaterThan(iHuge);
    expect(text).toContain('Langkah berikut: list_api_endpoints');
  });

  test('P3 list_api_endpoints tanpa filter memuat semua endpoint per spec', async ({ request }) => {
    const text = textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', {}));
    const groups = ['== billing ==', '== huge ==', '== petstore =='].map((g) => text.indexOf(g));
    expect(groups.every((i) => i >= 0)).toBe(true);
    expect([...groups].sort((a, b) => a - b)).toEqual(groups);
    expect(parseEndpointLines(text)).toHaveLength(A_ENDPOINTS);
    expect(text).toContain(`Endpoint 1-${A_ENDPOINTS} dari ${A_ENDPOINTS} | spec: semua`);
    expect(text).toContain('POST /pets — Create a pet [createPet] (pets) [deprecated]');
    expect(text).not.toContain('[ada');
  });

  test('P4 filter tag, query, dan spec', async ({ request }) => {
    const byTag = parseEndpointLines(textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', { tag: 'pets' })));
    expect(byTag).toHaveLength(3);
    expect(new Set(byTag.map((r) => r.group))).toEqual(new Set(['petstore']));

    const byQuery = parseEndpointLines(textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', { query: 'invoice' })));
    expect(byQuery.map((r) => r.operationId).sort()).toEqual(['listInvoices', 'payInvoice']);
    expect(new Set(byQuery.map((r) => r.group))).toEqual(new Set(['billing']));

    const text = textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', { spec: 'petstore', query: 'petId' }));
    const rows = parseEndpointLines(text);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ method: 'GET', path: '/pets/{petId}' });
    expect(text).toContain('Endpoint 1-1 dari 1 | spec: petstore | query: "petId"');
  });

  test('P5 paginasi limit lalu offset tanpa duplikat', async ({ request }) => {
    const first = textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', { tag: 'pets', limit: 2 }));
    expect(parseEndpointLines(first)).toHaveLength(2);
    expect(first).toContain('[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2');
    const off = nextOffset(first);
    expect(off).toBe(2);
    const second = textOf(await callTool(request, env.kA.rawKey, 'list_api_endpoints', { tag: 'pets', limit: 2, offset: off }));
    const all = [...parseEndpointLines(first), ...parseEndpointLines(second)];
    expect(all).toHaveLength(3);
    expect(new Set(all.map((r) => `${r.method} ${r.path}`)).size).toBe(3);
    expect(nextOffset(second)).toBeNull();
  });

  test('P6 get_api_endpoint membuka skema, menandai sirkular, tanpa komponen tak terpakai', async ({ request }) => {
    const r = await callTool(request, env.kA.rawKey, 'get_api_endpoint', DETAIL_PETSTORE);
    expect(r.isError).toBe(false);
    const text = textOf(r);
    const d = parseDetail(text);
    expect(d.method).toBe('GET');
    expect(d.path).toBe('/pets/{petId}');
    expect(d.truncated).toBe(false);
    expect(d.schema_depth).toBe(4);
    const op = JSON.stringify(d.operation);
    expect(op).toContain('"owner"');
    expect(op).toContain('"address"');
    expect(op).toContain('"$circular":"#/components/schemas/Pet"');
    expect(text).not.toContain('Unused');
    expect(d.markers?.circular).toContain('#/components/schemas/Pet');
    expect(d.markers?.legend.length).toBeGreaterThan(0);
    expect((d.servers[0] as { url: string }).url).toBe('https://petstore.noobee.id/v1');
    expect((d.securitySchemes.apiKey as { name: string }).name).toBe('X-API-Key');
  });

  test('P7 operation_id sama dengan method dan path', async ({ request }) => {
    const byPath = parseDetail(textOf(await callTool(request, env.kA.rawKey, 'get_api_endpoint', DETAIL_PETSTORE)));
    const byOp = parseDetail(textOf(await callTool(request, env.kA.rawKey, 'get_api_endpoint', { spec: 'petstore', operation_id: 'showPetById' })));
    expect(byOp).toEqual(byPath);
  });

  test('P8 respons besar dipotong menjadi JSON valid dengan hint', async ({ request }) => {
    const r = await callTool(request, env.kA.rawKey, 'get_api_endpoint', { spec: 'huge', method: 'GET', path: '/report', schema_depth: 8 });
    expect(r.isError).toBe(false);
    const text = textOf(r);
    expect(text).not.toContain('[truncated: respons dipotong');
    const d = parseDetail(text);
    expect(d.truncated).toBe(true);
    expect(d.schema_depth).toBeLessThan(8);
    expect(d.hint).toContain('schema_depth');
    expect(Buffer.byteLength(text)).toBeLessThanOrEqual(102400);
  });

  test('P9 project dengan satu spec tidak perlu argumen spec', async ({ request }) => {
    for (const args of [{ method: 'GET', path: '/health' }, { operation_id: 'healthCheck' }]) {
      const r = await callTool(request, env.kB.rawKey, 'get_api_endpoint', args);
      expect(r.isError).toBe(false);
      expect(parseDetail(textOf(r)).spec).toBe('solo');
    }
  });

  test('P10 paginasi 120 endpoint dalam dua halaman', async ({ request }) => {
    const first = textOf(await callTool(request, env.kC.rawKey, 'list_api_endpoints', { spec: 'catalog' }));
    expect(parseEndpointLines(first)).toHaveLength(100);
    expect(nextOffset(first)).toBe(100);
    const second = textOf(await callTool(request, env.kC.rawKey, 'list_api_endpoints', { spec: 'catalog', offset: 100 }));
    expect(parseEndpointLines(second)).toHaveLength(20);
    expect(nextOffset(second)).toBeNull();
    expect(second).toContain('Endpoint 101-120 dari 120');
    const paths = [...parseEndpointLines(first), ...parseEndpointLines(second)].map((r) => r.path);
    expect(new Set(paths).size).toBe(120);
  });

  test('P11 project tanpa spec memberi pesan kosong yang menunjuk list_files', async ({ request }) => {
    const empty =
      'Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.';
    for (const name of ['list_api_specs', 'list_api_endpoints']) {
      const r = await callTool(request, env.kD.rawKey, name, {});
      expect(r.isError, name).toBe(false);
      expect(textOf(r)).toBe(empty);
    }
  });

  test('P12 endpoint yang dilihat agent sama dengan tab API Spec', async () => {
    const page = await env.admin.newPage();
    await page.goto(`/projects/${env.A.id}/api-specs/petstore`);
    const nav = page.getByRole('navigation', { name: 'Daftar endpoint' });
    await expect(nav).toBeVisible();
    await expect(nav.getByText('/pets/{petId}')).toBeVisible();
    await expect(nav.getByText('/pets', { exact: true }).first()).toBeVisible();
    await expect(nav.getByRole('link')).toHaveCount(3);
    await page.getByRole('link', { name: /Info for a specific pet/ }).click();
    await expect(page.getByText('showPetById').first()).toBeVisible();
    await page.close();
  });

  test.describe('argumen salah', () => {
    const invalid = async (request: Parameters<typeof callTool>[0], name: string, args: unknown, ...phrases: string[]) => {
      const r = await callTool(request, env.kA2.rawKey, name, args);
      expect(r.isError, JSON.stringify(args)).toBe(true);
      for (const p of phrases) expect(textOf(r), JSON.stringify(args)).toContain(p);
      return textOf(r);
    };

    test('N1 spec wajib bila project punya banyak spec', async ({ request }) => {
      const text = await invalid(request, 'get_api_endpoint', { method: 'GET', path: '/pets' });
      expect(text).toBe(
        'Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs.',
      );
    });

    test('N2 spec tidak ditemukan menyebut spec valid', async ({ request }) => {
      await invalid(request, 'list_api_endpoints', { spec: 'petstor' }, 'Spec tidak ditemukan: petstor', 'billing, huge, petstore');
    });

    test('N3 endpoint tidak ditemukan memberi saran', async ({ request }) => {
      await invalid(
        request,
        'get_api_endpoint',
        { spec: 'petstore', method: 'GET', path: '/pet/{petId}' },
        'Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}',
        'Similar:',
        '- GET /pets/{petId}',
        'list_api_endpoints',
      );
    });

    test('N4 operation_id salah huruf memberi saran', async ({ request }) => {
      const text = await invalid(request, 'get_api_endpoint', { spec: 'petstore', operation_id: 'showPetByID' }, 'Similar:', 'showPetById');
      expect(text.split('\n')[0]).toContain(': operation_id showPetByID');
    });

    test('N5 argumen project ditolak dan project tetap dari key', async ({ request }) => {
      // Backend menyebut nama argumen persis seperti yang dikirim agent (project_id / projectId / project).
      const msg = (arg: string) => `Argumen ${arg} tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.`;
      const calls: [string, string, unknown][] = [
        ['list_api_specs', 'project_id', { project_id: env.B.id }],
        ['list_api_endpoints', 'projectId', { projectId: env.B.id }],
        ['get_api_endpoint', 'project', { project: env.B.id, method: 'GET', path: '/health' }],
      ];
      for (const [name, arg, args] of calls) {
        const text = await invalid(request, name, args);
        expect(text).toBe(msg(arg));
        expect(text).not.toMatch(/solo|Ledger/);
      }
      await invalid(request, 'get_api_endpoint', { spec: 'solo', method: 'GET', path: '/health' }, 'Spec tidak ditemukan: solo');
    });

    test('N6 batas limit dan offset', async ({ request }) => {
      await invalid(request, 'list_api_endpoints', { limit: 500 }, 'Argumen tidak valid: limit harus 1-300 (diterima: 500)');
      await invalid(request, 'list_api_endpoints', { limit: 0 }, 'limit harus 1-300 (diterima: 0)');
      await invalid(request, 'list_api_endpoints', { offset: -1 }, 'offset harus 0-1000000');
      await invalid(request, 'list_api_endpoints', { limit: '10' }, 'limit harus integer');
    });

    test('N7 offset melewati total', async ({ request }) => {
      await invalid(
        request,
        'list_api_endpoints',
        { spec: 'billing', offset: 5000 },
        'Argumen tidak valid: offset 5000 melewati total 2 endpoint. Gunakan offset 0 sampai 1.',
      );
    });

    test('N8 kombinasi argumen get_api_endpoint', async ({ request }) => {
      const cases: [unknown, string][] = [
        [{ spec: 'petstore' }, 'method wajib diisi bersama path, atau isi operation_id'],
        [{ spec: 'petstore', method: 'GET', path: '/pets', operation_id: 'listPets' }, 'pilih salah satu'],
        [{ spec: 'petstore', method: 'FETCH', path: '/pets' }, 'method harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE'],
        [{ spec: 'petstore', method: 'GET', path: 'pets' }, 'path harus diawali /'],
        [{ spec: 'petstore', operation_id: 'listPets', schema_depth: 9 }, 'schema_depth harus 1-8 (diterima: 9)'],
        [{ spec: 'petstore', method: 'GET' }, 'path wajib diisi bersama method'],
      ];
      for (const [args, phrase] of cases) {
        const text = await invalid(request, 'get_api_endpoint', args, phrase);
        expect(text.startsWith('Argumen tidak valid:')).toBe(true);
      }
    });

    test('N9 argumen tidak dikenal -> isError; arguments bukan object -> -32602 gateway', async ({ request }) => {
      await invalid(request, 'list_api_specs', { spec: 'petstore' }, 'spec tidak dikenal. Tool ini tidak menerima argumen.');
      await invalid(request, 'list_api_endpoints', { foo: 1 }, 'foo tidak dikenal. Argumen yang didukung: spec, tag, query, limit, offset.');
      const res = await mcpPost(request, env.kA2.rawKey, toolsCallBody(9001, 'list_api_specs', []));
      // Gateway T07 menolak arguments non-object sebelum tool dipanggil (-32602), tanpa audit.
      const err = await expectRpcFailure(res, 200, -32602);
      expect(err.message).toBe('Invalid params');
    });
  });

  test('N10 key dicabut ditolak gateway dan tidak tercatat di audit', async ({ request }) => {
    const k = await agentFor(env, env.A, 'dicabut');
    await revokeKeyApi(env.admin.request, env.A.id, k.keyId);
    await expectRpcFailure(await mcpPost(request, k.rawKey, toolsCallBody(1, 'list_api_specs', {})), 401, UNAUTHORIZED);
    const res = await env.admin.request.get('/api/audit-log', {
      params: { actor_type: 'api_key', actor_id: k.keyId, action: 'mcp.list_api_specs' },
    });
    expect(res.status()).toBe(200);
    expect(((await res.json()) as { items: unknown[] }).items).toHaveLength(0);
  });

  test('P13 jejak agent tampil di /audit-log tanpa isi skema', async () => {
    const page = await env.admin.newPage();
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.kA.keyId}`);
    await expect(page.getByRole('table')).toBeVisible();
    for (const action of ['mcp.list_api_specs', 'mcp.list_api_endpoints', 'mcp.get_api_endpoint']) {
      await expect(page.getByRole('row').filter({ hasText: action }).first()).toContainText(`${env.kA.keyName} (${env.kA.prefix})`);
    }

    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.kA.keyId}&action=mcp.get_api_endpoint`);
    const row = page.getByRole('row').filter({ hasText: 'petstore GET /pets/{petId}' }).first();
    await expect(row).toContainText('Berhasil');
    await row.getByRole('button').first().click();
    for (const field of ['found', 'schema_depth', 'circular_count', 'bytes']) {
      await expect(page.locator('pre').filter({ hasText: `"${field}"` }).first()).toBeVisible();
    }
    for (const leaked of ['Owner', 'Address', 'X-API-Key', 'Info for a specific pet']) {
      await expect(page.getByText(leaked)).toHaveCount(0);
    }

    const list = await env.admin.request.get('/api/audit-log', {
      params: { actor_type: 'api_key', actor_id: env.kA.keyId, action: 'mcp.list_api_specs' },
    });
    const items = ((await list.json()) as { items: { id: string }[] }).items;
    expect(items.length).toBeGreaterThan(0);
    const detail = await env.admin.request.get(`/api/audit-log/${items[0].id}`);
    const body = (await detail.json()) as { result: { count: number; endpoint_count: number; bytes: number } };
    expect(body.result.count).toBe(3);
    expect(body.result.endpoint_count).toBe(A_ENDPOINTS);
    expect(body.result.bytes).toBeGreaterThan(0);
    await page.close();
  });

  test('N11 kegagalan tool tercatat Gagal dengan found=false dan similar_count', async () => {
    const page = await env.admin.newPage();
    await page.goto(`/audit-log?actor_type=api_key&actor_id=${env.kA2.keyId}&action=mcp.get_api_endpoint`);
    const row = page.getByRole('row').filter({ hasText: 'petstore GET /pet/{petId}' }).first();
    await expect(row).toContainText('Gagal');
    await row.getByRole('button').first().click();
    // Gateway T07 (audit.go) membuang error_code dari Summary tool, jadi hanya found/similar_count yang tercatat.
    await expect(page.locator('pre').filter({ hasText: '"found": false' }).first()).toBeVisible();
    await expect(page.getByText('similar_count').first()).toBeVisible();
    await expect(page.getByText('Info for a specific pet')).toHaveCount(0);
    await page.close();
  });
});

