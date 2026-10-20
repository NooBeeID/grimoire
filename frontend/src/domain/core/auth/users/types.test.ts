import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { toUserFormError, UserListSchema, UserSchema, UserWithTempPasswordSchema } from './types';

const USER = {
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: true, created_at: '2026-10-21T09:00:00Z', created_by: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d',
  created_by_label: 'admin@tim.id', updated_at: '2026-10-21T09:00:00Z', last_login_at: null,
};
const err = (status: number, code: string, details?: unknown) => new ApiError({ status, code, message: 'm', details });

describe('schemas', () => {
  it('accept contract examples', () => {
    expect(UserSchema.safeParse(USER).success).toBe(true);
    expect(UserListSchema.safeParse({ items: [USER], total: 1, limit: 20, offset: 0 }).success).toBe(true);
    expect(UserWithTempPasswordSchema.safeParse({ user: USER, temporary_password: null }).success).toBe(true);
    expect(UserWithTempPasswordSchema.safeParse({ user: USER, temporary_password: 'Kq7mZp3RtX9aHw2N' }).success).toBe(true);
  });
  it('reject bad shapes', () => {
    expect(UserSchema.safeParse({ ...USER, role: 'root' }).success).toBe(false);
    expect(UserSchema.safeParse({ ...USER, id: 'x' }).success).toBe(false);
    expect(UserWithTempPasswordSchema.safeParse({ user: USER, temporary_password: '' }).success).toBe(false);
  });
});

describe('toUserFormError', () => {
  it('400 splits "_" into the form message', () => {
    expect(toUserFormError(err(400, 'INVALID_ARGUMENT', { fields: { email: 'salah', _: 'umum' } }))).toEqual({
      kind: 'form', fields: { email: 'salah' }, message: 'umum',
    });
  });
  it('409 last super admin vs email taken', () => {
    expect(toUserFormError(err(409, 'CONFLICT', { reason: 'LAST_SUPER_ADMIN' }))).toMatchObject({ message: 'Minimal harus ada satu super admin aktif' });
    expect(toUserFormError(err(409, 'CONFLICT'))).toEqual({ kind: 'form', fields: { email: 'Email sudah terdaftar' } });
  });
  it('403 self vs other', () => {
    expect(toUserFormError(err(403, 'FORBIDDEN', { reason: 'SELF_ACTION_FORBIDDEN' }))).toMatchObject({ message: expect.stringContaining('akun sendiri') });
    expect(toUserFormError(err(403, 'FORBIDDEN'))).toMatchObject({ message: 'Anda tidak memiliki akses' });
  });
  it('404 and generic', () => {
    expect(toUserFormError(err(404, 'NOT_FOUND'))).toEqual({ kind: 'notFound' });
    expect(toUserFormError(err(500, 'INTERNAL')).kind).toBe('generic');
    expect(toUserFormError(err(400, 'INVALID_ARGUMENT')).kind).toBe('generic');
  });
});
