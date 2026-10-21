import { expect, test, type APIRequestContext } from "@playwright/test";
import { spawnSync } from "node:child_process";
import { CSRF, uniqueEmail } from "./auth";

// Helper e2e T12 (undangan). Kedaluwarsa tidak bisa dicapai lewat config (INVITATION_TTL minimum 1 jam),
// jadi tes memanipulasi user_invitations langsung lewat psql.

const EMAIL_RE = /^[a-z0-9._+-]+@[a-z0-9.-]+$/;

export const dbAvailable = (): boolean => {
  if (!process.env.DATABASE_URL) return false;
  return spawnSync("psql", ["--version"], { encoding: "utf8" }).status === 0;
};

/** Lewati tes bila DATABASE_URL atau psql tidak tersedia. Panggil di test.beforeEach / awal tes. */
export function skipWithoutDb(): void {
  test.skip(
    !dbAvailable(),
    "butuh DATABASE_URL dan psql untuk memanipulasi user_invitations",
  );
}

function psql(sql: string): string {
  const r = spawnSync(
    "psql",
    [process.env.DATABASE_URL ?? "", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql],
    { encoding: "utf8" },
  );
  if (r.status !== 0)
    throw new Error(`psql gagal (${r.status}): ${r.stderr}${r.stdout}`);
  return r.stdout.trim();
}

function safeEmail(email: string): string {
  if (!EMAIL_RE.test(email))
    throw new Error(`email tidak aman untuk SQL: ${email}`);
  return email;
}

/** Membuat undangan akun email ini kedaluwarsa (expires_at 1 jam lalu). */
export function expireInvitationDb(email: string): void {
  const e = safeEmail(email);
  const out = psql(
    `WITH u AS (UPDATE user_invitations SET expires_at = now() - interval '1 hour'
       WHERE user_id = (SELECT id FROM users WHERE email = '${e}') RETURNING 1) SELECT count(*) FROM u`,
  );
  if (out !== "1") throw new Error(`undangan untuk ${e} tidak ditemukan`);
}

/** accepted_at undangan (ISO string) atau null bila belum diterima / tanpa baris. */
export function invitationAcceptedAtDb(email: string): string | null {
  const e = safeEmail(email);
  const out = psql(
    `SELECT COALESCE(accepted_at::text, '') FROM user_invitations WHERE user_id = (SELECT id FROM users WHERE email = '${e}')`,
  );
  return out === "" ? null : out;
}

export interface InvitedUser {
  id: string;
  email: string;
  password: string;
}

/** Admin mengundang email baru; server e2e tanpa MAIL_DRIVER sehingga respons membawa credentials (fallback). */
export async function inviteUserApi(
  admin: APIRequestContext,
  email = uniqueEmail("inv"),
): Promise<InvitedUser> {
  const res = await admin.post("/api/users/invite", {
    headers: CSRF,
    data: { email },
  });
  expect(res.status(), `invite ${email}: ${await res.text()}`).toBe(201);
  const body = (await res.json()) as {
    user: { id: string; email: string };
    credentials?: { temporary_password: string };
  };
  const password = body.credentials?.temporary_password ?? "";
  expect(password, "credentials fallback harus ada tanpa MAIL_DRIVER").not.toBe(
    "",
  );
  return { id: body.user.id, email: body.user.email, password };
}

export async function resendInvitationApi(
  admin: APIRequestContext,
  userId: string,
): Promise<string> {
  const res = await admin.post(`/api/users/${userId}/resend-invitation`, {
    headers: CSRF,
  });
  expect(res.status(), await res.text()).toBe(200);
  const body = (await res.json()) as {
    credentials?: { temporary_password: string };
  };
  const password = body.credentials?.temporary_password ?? "";
  expect(password).not.toBe("");
  return password;
}

/** Jumlah entri audit action tertentu untuk target (user id). */
export async function countAudit(
  admin: APIRequestContext,
  action: string,
  target: string,
): Promise<{ count: number; raw: string }> {
  let raw = "";
  let count = 0;
  await expect
    .poll(
      async () => {
        const res = await admin.get(
          `/api/audit-log?action=${encodeURIComponent(action)}&target=${encodeURIComponent(target)}&limit=200`,
        );
        expect(res.status()).toBe(200);
        raw = await res.text();
        count = ((JSON.parse(raw) as { items: unknown[] }).items ?? []).length;
        return true;
      },
      { timeout: 5_000 },
    )
    .toBe(true);
  return { count, raw };
}
