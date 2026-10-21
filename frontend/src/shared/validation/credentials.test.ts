import { describe, expect, it } from 'vitest';
import { EmailStatusSchema, minutesOf, rateLimitOf, TemporaryCredentialsSchema } from './credentials';

describe('credentials', () => {
  it('menerima kredensial valid dan menolak password kosong', () => {
    const ok = { email: 'rani@tim.id', temporary_password: 'Hm8tYq3RkW9vNcPz', login_url: '/login' };
    expect(TemporaryCredentialsSchema.safeParse(ok).success).toBe(true);
    expect(TemporaryCredentialsSchema.safeParse({ ...ok, temporary_password: '' }).success).toBe(false);
  });
  it('EmailStatusSchema menolak status asing', () => {
    expect(EmailStatusSchema.safeParse('queued').success).toBe(false);
    expect(EmailStatusSchema.safeParse('not_configured').success).toBe(true);
  });
  it('rateLimitOf membaca retry_after_seconds dan scope', () => {
    expect(rateLimitOf({ retry_after_seconds: 1840, scope: 'resend' })).toEqual({ seconds: 1840, scope: 'resend' });
    expect(rateLimitOf({ retry_after_seconds: 5 })).toBeNull();
    expect(rateLimitOf({ retry_after_seconds: 5, scope: 'lain' })).toBeNull();
    expect(rateLimitOf(null)).toBeNull();
  });
  it('minutesOf membulatkan ke atas dengan minimum 1', () => {
    expect(minutesOf(1840)).toBe(31);
    expect(minutesOf(0)).toBe(1);
  });
});
