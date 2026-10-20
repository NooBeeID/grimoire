import { expect, request as pwRequest, type APIRequestContext, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { z } from 'zod';

export const ADMIN_EMAIL = 'e2e-admin@tim.id';
export const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? 'e2e-admin-password-1';
export const STRONG_PASSWORD = 'Rahasia-Panjang-123!';
export const CSRF = { 'X-Requested-With': 'grimoire' };

const baseURL = () => process.env.E2E_BASE_URL ?? 'http://localhost:8080';

const UserResponse = z.object({
  user: z.object({ id: z.string(), email: z.string(), name: z.string(), role: z.string() }),
  temporary_password: z.string().nullable().optional(),
});

export const uniqueEmail = (prefix = 'u') => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@tim.id`;

/** Login lewat form UI; menunggu sampai meninggalkan /login. */
export async function loginUi(page: Page, email: string, password: string): Promise<void> {
  await page.goto('/login');
  await submitLogin(page, email, password);
}

export async function submitLogin(page: Page, email: string, password: string): Promise<void> {
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: /^(Masuk|Sign in)$/ }).click();
}

/** Login lewat API; cookie sesi tersimpan di context `request`. */
export async function apiLogin(request: APIRequestContext, email: string, password: string) {
  const res = await request.post('/api/auth/login', { headers: CSRF, data: { email, password } });
  expect(res.status(), `login ${email}`).toBe(200);
  return (await res.json()) as { user: { id: string; email: string; name: string; role: string; must_change_password: boolean } };
}

export interface CreatedUser {
  id: string;
  email: string;
  name: string;
  role: string;
  password: string;
}

/** Buat user lewat API admin. `request` harus sudah login sebagai super_admin. Tanpa `password` server membangkitkannya. */
export async function createUserApi(
  request: APIRequestContext,
  opts: { role?: 'developer' | 'super_admin'; password?: string; email?: string; name?: string } = {},
): Promise<CreatedUser> {
  const email = opts.email ?? uniqueEmail();
  const name = opts.name ?? 'E2E User';
  const data: Record<string, string> = { email, name, role: opts.role ?? 'developer' };
  if (opts.password) data.password = opts.password;
  const res = await request.post('/api/users', { headers: CSRF, data });
  expect(res.status(), `create ${email}: ${await res.text()}`).toBe(201);
  const body = UserResponse.parse(await res.json());
  const password = opts.password ?? body.temporary_password ?? '';
  expect(password).not.toBe('');
  return { id: body.user.id, email: body.user.email, name: body.user.name, role: body.user.role, password };
}

/** User yang sudah melewati ganti password wajib (flag false). Mengembalikan password final. */
export async function createReadyUserApi(
  adminRequest: APIRequestContext,
  opts: { role?: 'developer' | 'super_admin'; name?: string } = {},
): Promise<CreatedUser> {
  const u = await createUserApi(adminRequest, opts);
  const ctx = await pwRequest.newContext({ baseURL: baseURL() });
  try {
    await apiLogin(ctx, u.email, u.password);
    const res = await ctx.post('/api/auth/change-password', {
      headers: CSRF,
      data: { current_password: u.password, new_password: STRONG_PASSWORD },
    });
    expect(res.status(), await res.text()).toBe(204);
  } finally {
    await ctx.dispose();
  }
  return { ...u, password: STRONG_PASSWORD };
}

/** Context browser yang sudah login sebagai admin e2e, dengan izin clipboard. */
export async function adminContext(browser: Browser): Promise<BrowserContext> {
  const context = await browser.newContext({
    baseURL: baseURL(),
    permissions: ['clipboard-read', 'clipboard-write'],
  });
  await apiLogin(context.request, ADMIN_EMAIL, ADMIN_PASSWORD);
  return context;
}

/** Context browser baru tanpa sesi. */
export function newContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ baseURL: baseURL() });
}
