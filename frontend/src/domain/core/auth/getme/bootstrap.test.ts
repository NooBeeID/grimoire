import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { loadSession } from './bootstrap';
import type { Me } from './types';

const me: Me = { id: '1', email: 'a@b.c', name: 'A', role: 'developer', must_change_password: false, email_enabled: false };
const ident = { userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: false };
const err = (status: number, code = 'X') => new ApiError({ status, code, message: 'm' });

describe('loadSession', () => {
  it.each([true, false])('mengisi identity dan mailStatus=%s', async (enabled) => {
    const setIdentity = vi.fn();
    const setMailStatus = vi.fn();
    await loadSession({ getMe: async () => ({ ...me, email_enabled: enabled }), setIdentity, setMailStatus });
    expect(setIdentity).toHaveBeenCalledWith(ident);
    expect(setMailStatus).toHaveBeenCalledWith(enabled);
  });
  it('401 -> anonymous tanpa throw dan mailStatus null', async () => {
    const setIdentity = vi.fn();
    const setMailStatus = vi.fn();
    await loadSession({ getMe: async () => Promise.reject(err(401)), setIdentity, setMailStatus });
    expect(setIdentity).toHaveBeenCalledWith(null);
    expect(setMailStatus).toHaveBeenCalledWith(null);
  });
  it.each([err(500), err(0, 'NETWORK'), err(200, 'INVALID_RESPONSE')])('throws dan store tak berubah pada %j', async (e) => {
    const setIdentity = vi.fn();
    const setMailStatus = vi.fn();
    await expect(loadSession({ getMe: async () => Promise.reject(e), setIdentity, setMailStatus })).rejects.toBe(e);
    expect(setIdentity).not.toHaveBeenCalled();
    expect(setMailStatus).not.toHaveBeenCalled();
  });
});
