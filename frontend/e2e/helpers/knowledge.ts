import { cpSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { CSRF } from './auth';

export const FIXTURE_DOCS = resolve(process.cwd(), 'e2e/fixtures/knowledge/docs');

/**
 * Salinan fixture `docs/` di direktori sementara. `.git/config` dibuat di sini karena git
 * tidak bisa menyimpan path berkomponen `.git` di repo.
 */
export function stageDocs(): { root: string; docs: string } {
  const root = mkdtempSync(join(tmpdir(), 'knowledge-e2e-'));
  const docs = join(root, 'docs');
  cpSync(FIXTURE_DOCS, docs, { recursive: true });
  mkdirSync(join(docs, '.git'), { recursive: true });
  writeFileSync(join(docs, '.git', 'config'), '[core]\n\trepositoryformatversion = 0\n');
  return { root, docs };
}

export interface ApiFile {
  path: string;
  content: string | Buffer;
}

export interface UploadApiResult {
  summary: Record<'created' | 'updated' | 'unchanged' | 'skipped' | 'rejected', number>;
  items: { path: string; status: string; reason?: string }[];
}

/** Upload multipart lewat API (melewati validasi klien). */
export async function uploadApi(request: APIRequestContext, projectId: string, files: ApiFile[]): Promise<UploadApiResult> {
  const form = new FormData();
  for (const f of files) form.append('files', new Blob([new Uint8Array(typeof f.content === 'string' ? Buffer.from(f.content) : f.content)], { type: 'text/markdown' }), f.path);
  const res = await request.post(`/api/projects/${projectId}/knowledge/upload`, { headers: CSRF, multipart: form });
  expect(res.status(), `upload: ${await res.text()}`).toBe(200);
  return (await res.json()) as UploadApiResult;
}

export async function getFileApi(request: APIRequestContext, projectId: string, path: string) {
  const res = await request.get(`/api/projects/${projectId}/knowledge/files`, { params: { path } });
  return { status: res.status(), body: res.status() === 200 ? ((await res.json()) as { content: string; version: string }) : null };
}

export async function openKnowledge(page: Page, projectId: string, query = ''): Promise<void> {
  await page.goto(`/projects/${projectId}/knowledge${query ? (query.startsWith('?') ? query : `?${query}`) : ''}`);
}

/** Gagalkan test bila ada pelanggaran CSP di konsol. */
export function watchCsp(page: Page): string[] {
  const violations: string[] = [];
  page.on('console', (m) => {
    if (m.text().includes('Refused to')) violations.push(m.text());
  });
  return violations;
}
