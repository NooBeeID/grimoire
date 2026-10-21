import { expect, type APIRequestContext } from '@playwright/test';
import { CSRF } from './auth';

export interface CreatedProjectApi {
  id: string;
  name: string;
}

/** Nama project unik per test agar paralel aman. */
export const uniqueProjectName = (prefix = 'P') => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

export async function createProjectApi(request: APIRequestContext, name = uniqueProjectName(), description = ''): Promise<CreatedProjectApi> {
  const res = await request.post('/api/projects', { headers: CSRF, data: { name, description } });
  expect(res.status(), `create project: ${await res.text()}`).toBe(201);
  const body = (await res.json()) as { id: string; name: string };
  return { id: body.id, name: body.name };
}

export async function addMemberApi(request: APIRequestContext, projectId: string, userId: string, role: 'owner' | 'member' = 'member') {
  const res = await request.post(`/api/projects/${projectId}/members`, { headers: CSRF, data: { user_id: userId, role } });
  expect(res.status(), `add member: ${await res.text()}`).toBe(201);
}

export async function deleteProjectApi(request: APIRequestContext, id: string, name: string) {
  const res = await request.delete(`/api/projects/${id}`, { headers: CSRF, data: { confirm_name: name } });
  expect(res.status(), `delete project: ${await res.text()}`).toBe(204);
}
