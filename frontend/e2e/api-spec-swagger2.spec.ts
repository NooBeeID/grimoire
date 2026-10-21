import { expect, test, type BrowserContext } from '@playwright/test';
import { readSpecFixture, uploadSpecsApi, uploadSpecsRaw } from './helpers/apispecs';
import { CSRF, adminContext } from './helpers/auth';
import { createProjectApi, uniqueProjectName } from './helpers/projects';

// E2E T10 (Swagger 2.0) lewat API nyata: upload, detail apa adanya, unduh byte identik, penolakan.

test.describe.configure({ mode: 'serial' });

let admin: BrowserContext;
let projectId: string;

test.beforeAll(async ({ browser }) => {
  admin = await adminContext(browser, 'id-ID');
  projectId = (await createProjectApi(admin.request, uniqueProjectName('hr-platform-e2e'))).id;
});

test.afterAll(async () => {
  await admin.close();
});

test('P1 upload 2.0 -> created dengan versi 2.0', async () => {
  const res = await uploadSpecsApi(admin.request, projectId, [readSpecFixture('swagger-2.0.yaml')]);
  expect(res.summary.created).toBe(1);
  expect(res.results[0]).toMatchObject({ name: 'swagger-2.0', status: 'created', endpoint_count: 4 });
});

test('P3 detail memuat dokumen 2.0 apa adanya', async () => {
  const res = await admin.request.get(`/api/projects/${projectId}/api-specs/swagger-2.0`);
  expect(res.status()).toBe(200);
  const body = (await res.json()) as { openapi_version: string; document: Record<string, unknown> };
  expect(body.openapi_version).toBe('2.0');
  expect(body.document.swagger).toBe('2.0');
  expect(body.document.openapi).toBeUndefined();
});

test('P6 JSON diunggah dan unduhan /raw identik byte', async () => {
  const file = readSpecFixture('swagger-2.0.json', 'petstore-json.json');
  const res = await uploadSpecsApi(admin.request, projectId, [file]);
  expect(res.results[0].status).toBe('created');
  const raw = await admin.request.get(`/api/projects/${projectId}/api-specs/petstore-json/raw`);
  expect(Buffer.from(await raw.body()).equals(Buffer.from(file.content))).toBe(true);
});

test('P7 unggah ulang file sama -> unchanged', async () => {
  const res = await uploadSpecsRaw(admin.request, projectId, [readSpecFixture('swagger-2.0.json', 'petstore-json.json')]);
  expect(res.results[0].status).toBe('unchanged');
});

test('P8 validate saja -> valid, tidak tersimpan', async () => {
  const f = readSpecFixture('swagger-2.0.yaml', 'validasi-saja.yaml');
  const form = new FormData();
  form.append('files', new Blob([new Uint8Array(Buffer.from(f.content))], { type: 'application/yaml' }), f.file);
  const res = await admin.request.post(`/api/projects/${projectId}/api-specs/validate`, {
    headers: CSRF,
    multipart: form,
  });
  expect(res.status(), await res.text()).toBe(200);
  const list = await admin.request.get(`/api/projects/${projectId}/api-specs`);
  expect(JSON.stringify(await list.json())).not.toContain('validasi-saja');
});

test('N1 ref hilang -> rejected UNRESOLVED_REF dengan pointer dan baris', async () => {
  const res = await uploadSpecsRaw(admin.request, projectId, [readSpecFixture('swagger-2.0-unresolved-ref.yaml')]);
  expect(res.summary.rejected).toBe(1);
  expect(res.results[0].reason_code).toBe('INVALID_SPEC');
  const issues = (res.results[0] as unknown as { issues: { code: string; pointer: string; line: number }[] }).issues;
  expect(issues[0]).toMatchObject({ code: 'UNRESOLVED_REF' });
  expect(issues[0].pointer).toContain('#/');
  expect(issues[0].line).toBeGreaterThan(0);
});

test('N2 swagger 1.2 -> rejected UNSUPPORTED_VERSION di #/swagger', async () => {
  const res = await uploadSpecsRaw(admin.request, projectId, [readSpecFixture('swagger-1.2.yaml')]);
  const issues = (res.results[0] as unknown as { issues: { code: string; pointer: string }[] }).issues;
  expect(res.results[0].status).toBe('rejected');
  expect(issues[0]).toMatchObject({ code: 'UNSUPPORTED_VERSION', pointer: '#/swagger' });
});
