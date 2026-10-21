import {
  expect,
  request as pwRequest,
  test,
  type APIRequestContext,
} from "@playwright/test";
import { ErrorEnvelopeForE2E } from "./envelope";
import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  CSRF,
  STRONG_PASSWORD,
  apiLogin,
  createReadyUserApi,
  loginUi,
  uniqueEmail,
} from "./helpers/auth";
import {
  countAudit,
  expireInvitationDb,
  invitationAcceptedAtDb,
  inviteUserApi,
  resendInvitationApi,
  skipWithoutDb,
} from "./helpers/invitations";

// E2E T12 core/auth: login menolak undangan kedaluwarsa identik kredensial salah; change-password
// menandai undangan diterima dan mencatat audit user.invitation.accepted.
// Server e2e tanpa MAIL_DRIVER (noop) sehingga undangan mengembalikan credentials.temporary_password.

const baseURL = () =>
  test.info().project.use.baseURL ??
  process.env.E2E_BASE_URL ??
  "http://localhost:8080";

async function newAnon(): Promise<APIRequestContext> {
  return pwRequest.newContext({ baseURL: baseURL() });
}

async function newAdmin(): Promise<APIRequestContext> {
  const ctx = await newAnon();
  await apiLogin(ctx, ADMIN_EMAIL, ADMIN_PASSWORD);
  return ctx;
}

async function rawLogin(
  ctx: APIRequestContext,
  email: string,
  password: string,
) {
  return ctx.post("/api/auth/login", {
    headers: CSRF,
    data: { email, password },
  });
}

async function strippedBody(res: Awaited<ReturnType<typeof rawLogin>>) {
  const body = ErrorEnvelopeForE2E.parse(await res.json());
  const { trace_id: _t, ...rest } = body;
  return rest;
}

function expectNoSession(res: Awaited<ReturnType<typeof rawLogin>>) {
  expect(res.headers()["set-cookie"] ?? "").not.toContain("grimoire_session");
  expect(res.headers()["retry-after"]).toBeUndefined();
}

test.describe("undangan: login dan terima", () => {
  test.beforeEach(() => skipWithoutDb());

  test("positif 1: undangan diterima lewat change-password, audit tepat satu, tanpa penanda menunggu", async () => {
    const admin = await newAdmin();
    const user = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      const me = await apiLogin(user, inv.email, inv.password);
      expect(me.user.must_change_password).toBe(true);
      const gated = await user.get("/api/users");
      expect(gated.status()).toBe(403);
      expect(ErrorEnvelopeForE2E.parse(await gated.json()).error.code).toBe(
        "PASSWORD_CHANGE_REQUIRED",
      );
      expect(invitationAcceptedAtDb(inv.email)).toBeNull();

      const ch = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: { current_password: inv.password, new_password: STRONG_PASSWORD },
      });
      expect(ch.status()).toBe(204);
      const after = (await (await user.get("/api/auth/me")).json()) as {
        user: { must_change_password: boolean };
      };
      expect(after.user.must_change_password).toBe(false);
      expect(invitationAcceptedAtDb(inv.email)).not.toBeNull();

      const detail = (await (
        await admin.get(`/api/users/${inv.id}`)
      ).json()) as { user?: { invitation?: unknown }; invitation?: unknown };
      expect((detail.user ?? detail).invitation ?? null).toBeNull();

      const audit = await countAudit(admin, "user.invitation.accepted", inv.id);
      expect(audit.count).toBe(1);
      expect(audit.raw).not.toContain(inv.password);
      expect(audit.raw).not.toContain(STRONG_PASSWORD);
    } finally {
      await user.dispose();
      await admin.dispose();
    }
  });

  test("positif 2: login UI penuh akun undangan sampai ganti password", async ({
    page,
  }) => {
    const admin = await newAdmin();
    try {
      const inv = await inviteUserApi(admin);
      await loginUi(page, inv.email, inv.password);
      await expect(page).toHaveURL(/\/change-password/);
      await page
        .getByLabel(/^(Password saat ini|Current password)/i)
        .fill(inv.password);
      await page
        .getByLabel(/^(Password baru|New password)/i)
        .first()
        .fill(STRONG_PASSWORD);
      const confirm = page.getByLabel(/(Konfirmasi|Confirm)/i);
      if (await confirm.count()) await confirm.first().fill(STRONG_PASSWORD);
      await page
        .getByRole("button", {
          name: /(Ganti|Ubah|Simpan|Change|Save|Update)/i,
        })
        .first()
        .click();
      await expect(page).not.toHaveURL(/\/(login|change-password)/);
      expect(invitationAcceptedAtDb(inv.email)).not.toBeNull();
    } finally {
      await admin.dispose();
    }
  });

  test("positif 3: kedaluwarsa dijawab identik password salah; resend memulihkan", async () => {
    const admin = await newAdmin();
    const anon = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      expireInvitationDb(inv.email);
      const expired = await rawLogin(anon, inv.email, inv.password);
      const wrong = await rawLogin(anon, inv.email, "salah-salah-1-xyz");
      expect(expired.status()).toBe(401);
      expect(wrong.status()).toBe(401);
      expectNoSession(expired);
      expectNoSession(wrong);
      const a = await strippedBody(expired);
      expect(a.error.code).toBe("INVALID_CREDENTIALS");
      expect(a).toEqual(await strippedBody(wrong));

      const newPw = await resendInvitationApi(admin, inv.id);
      expect((await rawLogin(anon, inv.email, inv.password)).status()).toBe(
        401,
      );
      expect((await rawLogin(anon, inv.email, newPw)).status()).toBe(200);
    } finally {
      await anon.dispose();
      await admin.dispose();
    }
  });

  test("positif 4: akun tanpa undangan tidak menghasilkan user.invitation.accepted", async () => {
    const admin = await newAdmin();
    try {
      const u = await createReadyUserApi(admin);
      const ctx = await newAnon();
      try {
        await apiLogin(ctx, u.email, u.password);
      } finally {
        await ctx.dispose();
      }
      expect(
        (await countAudit(admin, "user.invitation.accepted", u.id)).count,
      ).toBe(0);
    } finally {
      await admin.dispose();
    }
  });

  test("positif 5: undangan yang sudah diterima tetap bisa login walau expires_at lampau", async () => {
    const admin = await newAdmin();
    const user = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      await apiLogin(user, inv.email, inv.password);
      const ch = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: { current_password: inv.password, new_password: STRONG_PASSWORD },
      });
      expect(ch.status()).toBe(204);
      expireInvitationDb(inv.email);
      const again = await newAnon();
      try {
        await apiLogin(again, inv.email, STRONG_PASSWORD);
      } finally {
        await again.dispose();
      }
    } finally {
      await user.dispose();
      await admin.dispose();
    }
  });

  test("negatif 1: kedaluwarsa diulang 5 kali lalu 429 ACCOUNT_LOCKED dengan Retry-After", async () => {
    const admin = await newAdmin();
    const anon = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      expireInvitationDb(inv.email);
      for (let i = 1; i <= 5; i++) {
        const res = await rawLogin(anon, inv.email, inv.password);
        expect(res.status(), `percobaan ${i}`).toBe(401);
      }
      const locked = await rawLogin(anon, inv.email, inv.password);
      expect(locked.status()).toBe(429);
      expect(locked.headers()["retry-after"]).toBeTruthy();
      expect(ErrorEnvelopeForE2E.parse(await locked.json()).error.code).toBe(
        "ACCOUNT_LOCKED",
      );
    } finally {
      await anon.dispose();
      await admin.dispose();
    }
  });

  test("negatif 2: sesi lama ditolak change-password setelah undangan kedaluwarsa", async () => {
    const admin = await newAdmin();
    const user = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      await apiLogin(user, inv.email, inv.password);
      expireInvitationDb(inv.email);
      const ch = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: { current_password: inv.password, new_password: STRONG_PASSWORD },
      });
      expect(ch.status()).toBe(400);
      const body = ErrorEnvelopeForE2E.parse(await ch.json());
      expect(body.error.code).toBe("INVALID_ARGUMENT");
      expect(
        (body.error.details as { fields: Record<string, string> }).fields
          .current_password,
      ).toBeTruthy();
      expect(invitationAcceptedAtDb(inv.email)).toBeNull();
      const me = (await (await user.get("/api/auth/me")).json()) as {
        user: { must_change_password: boolean };
      };
      expect(me.user.must_change_password).toBe(true);

      const newPw = await resendInvitationApi(admin, inv.id);
      expect((await user.get("/api/auth/me")).status()).toBe(401);
      const fresh = await newAnon();
      try {
        await apiLogin(fresh, inv.email, newPw);
        const ok = await fresh.post("/api/auth/change-password", {
          headers: CSRF,
          data: { current_password: newPw, new_password: STRONG_PASSWORD },
        });
        expect(ok.status()).toBe(204);
      } finally {
        await fresh.dispose();
      }
    } finally {
      await user.dispose();
      await admin.dispose();
    }
  });

  test("negatif 3: kedaluwarsa + password salah sama dengan kedaluwarsa + password benar", async () => {
    const admin = await newAdmin();
    const anon = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      expireInvitationDb(inv.email);
      const right = await rawLogin(anon, inv.email, inv.password);
      const wrong = await rawLogin(anon, inv.email, "salah-salah-1-xyz");
      expect([right.status(), wrong.status()]).toEqual([401, 401]);
      expect(await strippedBody(right)).toEqual(await strippedBody(wrong));
    } finally {
      await anon.dispose();
      await admin.dispose();
    }
  });

  test("negatif 4: akun nonaktif dengan undangan kedaluwarsa -> 401 tanpa pembeda", async () => {
    const admin = await newAdmin();
    const anon = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      expireInvitationDb(inv.email);
      const off = await admin.post(`/api/users/${inv.id}/deactivate`, {
        headers: CSRF,
      });
      expect(off.status(), await off.text()).toBeLessThan(300);
      const res = await rawLogin(anon, inv.email, inv.password);
      expect(res.status()).toBe(401);
      expectNoSession(res);
      const wrong = await rawLogin(
        anon,
        uniqueEmail("none"),
        "salah-salah-1-xyz",
      );
      expect(await strippedBody(res)).toEqual(await strippedBody(wrong));
    } finally {
      await anon.dispose();
      await admin.dispose();
    }
  });

  test("negatif 5: change-password kedua tidak menambah audit user.invitation.accepted", async () => {
    const admin = await newAdmin();
    const user = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      await apiLogin(user, inv.email, inv.password);
      const first = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: { current_password: inv.password, new_password: STRONG_PASSWORD },
      });
      expect(first.status()).toBe(204);
      const second = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: {
          current_password: STRONG_PASSWORD,
          new_password: `${STRONG_PASSWORD}-2`,
        },
      });
      expect(second.status()).toBe(204);
      expect(
        (await countAudit(admin, "user.invitation.accepted", inv.id)).count,
      ).toBe(1);
    } finally {
      await user.dispose();
      await admin.dispose();
    }
  });

  test("negatif 6: password baru lemah pada undangan menunggu -> 400 dan tidak diterima", async () => {
    const admin = await newAdmin();
    const user = await newAnon();
    try {
      const inv = await inviteUserApi(admin);
      await apiLogin(user, inv.email, inv.password);
      const res = await user.post("/api/auth/change-password", {
        headers: CSRF,
        data: { current_password: inv.password, new_password: "pendek" },
      });
      expect(res.status()).toBe(400);
      const body = ErrorEnvelopeForE2E.parse(await res.json());
      expect(
        (body.error.details as { fields: Record<string, string> }).fields
          .new_password,
      ).toBeTruthy();
      expect(invitationAcceptedAtDb(inv.email)).toBeNull();
      expect(
        (await countAudit(admin, "user.invitation.accepted", inv.id)).count,
      ).toBe(0);
    } finally {
      await user.dispose();
      await admin.dispose();
    }
  });
});
