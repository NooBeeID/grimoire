import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';
import { VALID_TOKEN } from '../fixtures/fake-git-server';
import { CSRF } from './auth';

export const INVALID_TOKEN = 'ghp_notAValidToken0000';

/** Nama repo unik per tes; tes paralel berbagi satu server palsu. */
export const uniqueRepo = () => `noobee/docs-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

export const repoUrl = (repo: string, tail = '') => `https://github.com/${repo}${tail}`;

const OPENAPI_METHODS = ['get', 'post'] as const;

/** OpenAPI 3.0.3 valid dengan 14 operasi (7 path x 2 method). */
export function billingYaml(): string {
  const names = ['invoices', 'customers', 'subscriptions', 'plans', 'coupons', 'refunds', 'payouts'];
  const lines = ['openapi: 3.0.3', 'info:', '  title: Billing API', '  version: 1.0.0', 'paths:'];
  for (const n of names) {
    lines.push(`  /${n}:`);
    for (const m of OPENAPI_METHODS) {
      lines.push(`    ${m}:`, `      operationId: ${m}_${n}`, `      summary: ${m} ${n}`, '      responses:', "        '200':", '          description: ok');
    }
  }
  return `${lines.join('\n')}\n`;
}

/** Spec JSON dengan $ref eksternal di operasi /charges get; harus ditolak. */
export function paymentsJson(): string {
  return JSON.stringify({
    openapi: '3.0.3',
    info: { title: 'Payments API', version: '1.0.0' },
    paths: {
      '/charges': {
        get: {
          operationId: 'listCharges',
          responses: { '200': { description: 'ok', content: { 'application/json': { schema: { $ref: 'common.yaml#/components/schemas/Charge' } } } } },
        },
      },
    },
  });
}

export const docsRepoFiles = (): Record<string, string> => ({
  'docs/auth/login-flow.md': '# Alur login\n\nUser mengisi email dan password.\n',
  'docs/api/rate-limit.md': '# Rate limit\n\nBatas 100 permintaan per menit.\n',
  'docs/img/diagram.png': 'PNG-binary',
});

export async function saveTokenApi(request: APIRequestContext, token = VALID_TOKEN): Promise<void> {
  const res = await request.put('/api/me/git-credentials/github', { headers: CSRF, data: { token } });
  expect(res.status(), `save token: ${await res.text()}`).toBe(200);
}

export interface SyncApiBody {
  kind: 'knowledge' | 'api_spec';
  url: string;
  branch?: string;
  folder?: string;
  target_prefix?: string;
}

export async function runSyncApi(request: APIRequestContext, projectId: string, body: SyncApiBody) {
  const res = await request.post(`/api/projects/${projectId}/git-sync/run`, {
    headers: CSRF,
    data: { branch: '', folder: '', target_prefix: '', ...body },
  });
  return { status: res.status(), body: (await res.json()) as any };
}

export async function getSyncConfigApi(request: APIRequestContext, projectId: string, kind: 'knowledge' | 'api_spec' = 'knowledge') {
  const res = await request.get(`/api/projects/${projectId}/git-sync`, { params: { kind } });
  return { status: res.status(), body: res.status() === 200 ? ((await res.json()) as any) : null };
}

export const apiSpecsUrl = (projectId: string) => `/projects/${projectId}/api-specs`;

/** Buka dialog "Sync dari Git" dari tab yang sedang tampil. */
export async function openSyncDialog(page: Page): Promise<Locator> {
  await page.getByRole('button', { name: 'Sync dari Git' }).first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  return dialog;
}

export const urlInput = (dialog: Locator) => dialog.getByLabel('URL repo atau folder');
export const syncButton = (dialog: Locator) => dialog.getByRole('button', { name: 'Sync', exact: true });
export const rerunButton = (dialog: Locator) => dialog.getByRole('button', { name: 'Sync ulang', exact: true });
export const reportRow = (dialog: Locator, repoPath: string) => dialog.getByRole('row').filter({ hasText: repoPath });
