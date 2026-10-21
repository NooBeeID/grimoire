import { expect, type APIRequestContext } from '@playwright/test';
import { CSRF } from './auth';

export interface ApiKeyApi {
  id: string;
  name: string;
  prefix: string;
  status: string;
}

/** Nama key unik per test agar paralel aman. */
export const uniqueKeyName = (prefix = 'k') => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;

export async function createKeyApi(request: APIRequestContext, projectId: string, name = uniqueKeyName()) {
  const res = await request.post(`/api/projects/${projectId}/api-keys`, { headers: CSRF, data: { name } });
  expect(res.status(), `create key: ${await res.text()}`).toBe(201);
  return (await res.json()) as { key: ApiKeyApi; raw_key: string; mcp_url: string | null };
}

export async function revokeKeyApi(request: APIRequestContext, projectId: string, keyId: string) {
  const res = await request.post(`/api/projects/${projectId}/api-keys/${keyId}/revoke`, { headers: CSRF });
  expect(res.status(), `revoke key: ${await res.text()}`).toBe(200);
}

export async function listKeysApi(request: APIRequestContext, projectId: string) {
  const res = await request.get(`/api/projects/${projectId}/api-keys`);
  expect(res.status(), `list keys: ${await res.text()}`).toBe(200);
  return ((await res.json()) as { items: ApiKeyApi[] }).items;
}
