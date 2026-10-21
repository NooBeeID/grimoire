import { z } from 'zod';

// Mengikuti components/schemas/InvitationCredentials di api/modules/core/auth.yml.
export const TemporaryCredentialsSchema = z.object({
  email: z.string().min(1),
  temporary_password: z.string().min(1),
  login_url: z.string().min(1), // absolut, atau "/login" bila PUBLIC_URL kosong
});
export type TemporaryCredentials = z.infer<typeof TemporaryCredentialsSchema>;

export const EmailStatusSchema = z.enum(['sent', 'failed', 'not_configured', 'pending']);
export type EmailStatus = z.infer<typeof EmailStatusSchema>;

export const RateLimitScopeSchema = z.object({
  retry_after_seconds: z.number().int().nonnegative(),
  scope: z.enum(['invite', 'resend']),
});

export function rateLimitOf(details: unknown): { seconds: number; scope: 'invite' | 'resend' } | null {
  const r = RateLimitScopeSchema.safeParse(details);
  return r.success ? { seconds: r.data.retry_after_seconds, scope: r.data.scope } : null;
}

export function minutesOf(seconds: number): number {
  return Math.max(1, Math.ceil(seconds / 60));
}
