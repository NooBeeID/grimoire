import { expect, test, type Browser, type BrowserContext } from '@playwright/test';
import { z } from 'zod';
import { listKeysApi, revokeKeyApi, createKeyApi, uniqueKeyName } from './helpers/apikeys';
import { adminContext } from './helpers/auth';
import { createProjectApi } from './helpers/projects';
import {
  InitializeResult,
  MCP_VERSIONS,
  ToolsListResult,
  UNAUTHORIZED_BODY,
  expectRpcFailure,
  expectRpcResult,
  initializeBody,
  initializedNotification,
  mcpPost,
  mcpPostRaw,
  pingBody,
  randomWellFormedKey,
  toolsCallBody,
  toolsListBody,
} from './helpers/mcp';

// E2E T07 discovery/mcp-gateway terhadap binary sungguhan (butuh T01-T04 + T07).
// Panggilan MCP memakai fixture `request` tanpa cookie; jejak diverifikasi lewat UI audit log (admin) dan tab API Keys.
// Tool nyata (read_file, list_files) ditambah T08/T09; skenarionya test.fixme sampai merge.

interface Setup {
  admin: BrowserContext;
  projectId: string;
  projectName: string;
  keyId: string;
  keyName: string;
  prefix: string;
  rawKey: string;
}

async function setupKey(browser: Browser): Promise<Setup> {
  const admin = await adminContext(browser, 'id-ID');
  const proj = await createProjectApi(admin.request);
  const keyName = uniqueKeyName('mcp');
  const created = await createKeyApi(admin.request, proj.id, keyName);
  return {
    admin,
    projectId: proj.id,
    projectName: proj.name,
    keyId: created.key.id,
    keyName,
    prefix: created.key.prefix,
    rawKey: created.raw_key,
  };
}

const AuditList = z.object({ items: z.array(z.object({ action: z.string() }).passthrough()) });

async function countAudit(s: Setup): Promise<number> {
  const res = await s.admin.request.get(`/api/audit-log?actor_type=api_key&actor_id=${s.keyId}&limit=100`);
  expect(res.status()).toBe(200);
  return AuditList.parse(await res.json()).items.length;
}

async function openAudit(s: Setup, query: string) {
  const page = await s.admin.newPage();
  await page.goto(`/audit-log?${query}`);
  await expect(page.getByRole('table')).toBeVisible();
  return page;
}

test.describe('mcp-gateway: protokol', () => {
  test('P1 initialize -> initialized -> tools/list tercatat di audit log', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const init = InitializeResult.parse(await expectRpcResult(await mcpPost(request, s.rawKey, initializeBody(1))));
      expect(init.serverInfo.name).toBe('grimoire');
      expect(init.protocolVersion).toBe('2025-06-18');
      expect(init.instructions).toContain(s.projectName);

      const note = await mcpPost(request, s.rawKey, initializedNotification());
      expect(note.status()).toBe(202);
      expect(await note.text()).toBe('');

      ToolsListResult.parse(await expectRpcResult(await mcpPost(request, s.rawKey, toolsListBody(2))));

      const page = await openAudit(s, `actor_type=api_key&actor_id=${s.keyId}`);
      for (const action of ['mcp.initialize', 'mcp.tools_list']) {
        const row = page.getByRole('row').filter({ hasText: action });
        await expect(row).toHaveCount(1);
        await expect(row).toContainText('Berhasil');
        await expect(row).toContainText(`${s.keyName} (${s.prefix})`);
        await expect(row).toContainText(s.projectName);
      }
    } finally {
      await s.admin.close();
    }
  });

  test('P2 X-API-Key setara Bearer; P3 Bearer menang atas X-API-Key', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const viaHeader = await mcpPost(request, null, toolsListBody(1), { 'X-API-Key': s.rawKey });
      ToolsListResult.parse(await expectRpcResult(viaHeader));
      const mixed = await mcpPost(request, s.rawKey, toolsListBody(2), { 'X-API-Key': randomWellFormedKey().rawKey });
      expect(mixed.status()).toBe(200);
    } finally {
      await s.admin.close();
    }
  });

  test('P4 panggilan sukses mengisi last_used_at dan kolom Terakhir dipakai', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      await expectRpcResult(await mcpPost(request, s.rawKey, toolsListBody(1)));
      await expect
        .poll(async () => (await listKeysApi(s.admin.request, s.projectId)).find((k) => k.id === s.keyId)?.last_used_at, {
          timeout: 5000,
        })
        .not.toBeNull();
      const page = await s.admin.newPage();
      await page.goto(`/projects/${s.projectId}/api-keys`);
      const row = page.getByRole('row').filter({ hasText: s.keyName });
      await expect(row).toHaveCount(1);
    } finally {
      await s.admin.close();
    }
  });

  test('P5 ping mempertahankan id string; P6 semua versi protokol dinegosiasikan', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const res = await mcpPost(request, s.rawKey, pingBody('p-1'));
      expect(await res.json()).toEqual({ jsonrpc: '2.0', id: 'p-1', result: {} });
      for (const v of MCP_VERSIONS) {
        const init = InitializeResult.parse(await expectRpcResult(await mcpPost(request, s.rawKey, initializeBody(1, v))));
        expect(init.protocolVersion).toBe(v);
      }
    } finally {
      await s.admin.close();
    }
  });

  test('P7 Content-Type dengan charset dan header MCP-Protocol-Version diterima', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const res = await mcpPost(request, s.rawKey, toolsListBody(1), {
        'Content-Type': 'application/json; charset=utf-8',
        'MCP-Protocol-Version': '2025-06-18',
      });
      expect(res.status()).toBe(200);
    } finally {
      await s.admin.close();
    }
  });

  test('N6 Origin asing -> 403 tanpa jejak audit', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const before = await countAudit(s);
      const res = await mcpPost(request, s.rawKey, toolsListBody(1), { Origin: 'https://evil.example' });
      expect(res.status()).toBe(403);
      const body = z.object({ error: z.object({ code: z.literal('FORBIDDEN') }), trace_id: z.string().min(1) }).parse(await res.json());
      expect(body.error.code).toBe('FORBIDDEN');
      expect(await countAudit(s)).toBe(before);
    } finally {
      await s.admin.close();
    }
  });

  test('N7 GET dan DELETE -> 405 allow POST', async ({ request }) => {
    for (const method of ['GET', 'DELETE']) {
      const res = await request.fetch('/mcp', { method });
      expect(res.headers()['allow']).toBe('POST');
      await expectRpcFailure(res, 405, -32600);
    }
  });

  test('N8 metode tidak dikenal -> -32601', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const res = await mcpPost(request, s.rawKey, { jsonrpc: '2.0', id: 1, method: 'resources/list' });
      const err = await expectRpcFailure(res, 200, -32601);
      expect(err.data).toMatchObject({ method: 'resources/list' });
    } finally {
      await s.admin.close();
    }
  });

  test('N9 versi tidak didukung -> -32602 dan audit Gagal', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const err = await expectRpcFailure(await mcpPost(request, s.rawKey, initializeBody(1, '2024-11-05')), 200, -32602);
      expect(err.data).toMatchObject({ requested: '2024-11-05', supported: [...MCP_VERSIONS] });
      const page = await openAudit(s, `actor_type=api_key&actor_id=${s.keyId}&action=mcp.initialize`);
      await expect(page.getByRole('row').filter({ hasText: 'mcp.initialize' })).toContainText('Gagal');
    } finally {
      await s.admin.close();
    }
  });

  test('N10-N14 permintaan cacat ditolak dengan kode JSON-RPC yang tepat', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      await expectRpcFailure(
        await mcpPost(request, s.rawKey, toolsListBody(1), { 'Content-Type': 'text/plain' }),
        415,
        -32600,
      );
      const parse = await expectRpcFailure(await mcpPostRaw(request, s.rawKey, '{bukan json'), 400, -32700);
      expect(parse.message).toBe('Parse error');
      const batch = await expectRpcFailure(await mcpPost(request, s.rawKey, [initializeBody(1)]), 400, -32600);
      expect(batch.data).toMatchObject({ reason: 'batch not supported' });
      const big = initializeBody(1);
      big.params.clientInfo.name = 'x'.repeat(70 * 1024);
      const tooBig = await expectRpcFailure(await mcpPost(request, s.rawKey, big), 413, -32600);
      expect(tooBig.data).toMatchObject({ limit_bytes: 65536 });
      await expectRpcFailure(
        await mcpPost(request, s.rawKey, toolsListBody(1), { 'MCP-Protocol-Version': '1999-01-01' }),
        400,
        -32600,
      );
    } finally {
      await s.admin.close();
    }
  });
});

test.describe('mcp-gateway: autentikasi', () => {
  async function expectUnauthorized(res: Awaited<ReturnType<typeof mcpPost>>) {
    expect(res.status()).toBe(401);
    expect(res.headers()['www-authenticate']).toMatch(/^Bearer/);
    expect(await res.text()).toBe(UNAUTHORIZED_BODY);
  }

  test('N1 tanpa header auth -> 401 seragam', async ({ request }) => {
    await expectUnauthorized(await mcpPost(request, null, toolsListBody(1)));
  });

  test('N2 key acak berformat benar -> 401 dan jejak mcp.auth.rejected', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const { rawKey, prefix } = randomWellFormedKey();
      await expectUnauthorized(await mcpPost(request, rawKey, toolsListBody(1)));
      const page = await openAudit(s, `action=mcp.auth.rejected&target=${prefix}`);
      const row = page.getByRole('row').filter({ hasText: prefix });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText('Ditolak');
    } finally {
      await s.admin.close();
    }
  });

  test('N3 key dicabut -> 401 pada panggilan berikutnya', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      await expectRpcResult(await mcpPost(request, s.rawKey, toolsListBody(1)));
      await revokeKeyApi(s.admin.request, s.projectId, s.keyId);
      await expectUnauthorized(await mcpPost(request, s.rawKey, toolsListBody(2)));
      const page = await openAudit(s, `action=mcp.auth.rejected&target=${s.prefix}`);
      await expect(page.getByRole('row').filter({ hasText: s.prefix })).toHaveCount(1);
    } finally {
      await s.admin.close();
    }
  });

  test('N4 skema Basic -> 401; N5 Bearer acak + X-API-Key valid -> 401', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      await expectUnauthorized(
        await mcpPost(request, null, toolsListBody(1), { Authorization: 'Basic Zm9vOmJhcg==' }),
      );
      const res = await mcpPost(request, randomWellFormedKey().rawKey, toolsListBody(2), { 'X-API-Key': s.rawKey });
      expect(res.status()).toBe(401);
    } finally {
      await s.admin.close();
    }
  });
});

test.describe('mcp-gateway: panggilan tool', () => {
  test('N15 tool tidak dikenal -> -32602 dan baris mcp.unknown_tool', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const err = await expectRpcFailure(
        await mcpPost(request, s.rawKey, toolsCallBody(3, 'write_file', { path: 'docs/auth/login-flow.md' })),
        200,
        -32602,
      );
      expect(err.message).toBe('Unknown tool: write_file');
      const page = await openAudit(s, `actor_type=api_key&actor_id=${s.keyId}&action=mcp.unknown_tool`);
      const row = page.getByRole('row').filter({ hasText: 'write_file' });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText('Gagal');
    } finally {
      await s.admin.close();
    }
  });

  test('N16 params tools/call tidak valid -> -32602 tanpa audit', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const before = await countAudit(s);
      for (const body of [toolsCallBody(1, ''), toolsCallBody(2, 'x', 'str')]) {
        const err = await expectRpcFailure(await mcpPost(request, s.rawKey, body), 200, -32602);
        expect(err.message).toBe('Invalid params');
      }
      expect(await countAudit(s)).toBe(before);
    } finally {
      await s.admin.close();
    }
  });

  test.fixme('P9 read_file tercatat sekali tanpa isi file (menunggu T08)', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const res = await mcpPost(request, s.rawKey, toolsCallBody(1, 'read_file', { path: 'docs/auth/login-flow.md' }));
      const result = await expectRpcResult(res);
      expect(result.isError).toBe(false);
      const page = await openAudit(s, `actor_type=api_key&actor_id=${s.keyId}&action=mcp.read_file`);
      await expect(page.getByRole('row').filter({ hasText: 'docs/auth/login-flow.md' })).toHaveCount(1);
    } finally {
      await s.admin.close();
    }
  });

  test.fixme('P10 project_id asing di arguments diabaikan (menunggu T08)', async ({ browser, request }) => {
    const s = await setupKey(browser);
    try {
      const other = await createProjectApi(s.admin.request);
      const res = await mcpPost(
        request,
        s.rawKey,
        toolsCallBody(1, 'read_file', { path: 'docs/auth/login-flow.md', project_id: other.id }),
      );
      await expectRpcResult(res);
      const page = await openAudit(s, `actor_type=api_key&actor_id=${s.keyId}&action=mcp.read_file`);
      await expect(page.getByRole('row').filter({ hasText: s.projectName })).toHaveCount(1);
    } finally {
      await s.admin.close();
    }
  });
});
