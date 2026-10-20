import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import type { Identity } from '../../../../shared/stores/session';
import { loadSession } from './bootstrap';

const ident: Identity = { userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: false };
const err = (status: number, code = 'X') => new ApiError({ status, code, message: 'm' });

describe('loadSession', () => {
  it('sets identity on success', async () => {
    const setIdentity = vi.fn();
    await loadSession({ fetchMe: async () => ident, setIdentity });
    expect(setIdentity).toHaveBeenCalledWith(ident);
  });
  it('401 -> anonymous without throwing', async () => {
    const setIdentity = vi.fn();
    await loadSession({ fetchMe: async () => Promise.reject(err(401)), setIdentity });
    expect(setIdentity).toHaveBeenCalledWith(null);
  });
  it.each([err(500), err(0, 'NETWORK'), err(200, 'INVALID_RESPONSE')])('throws and leaves store untouched on %j', async (e) => {
    const setIdentity = vi.fn();
    await expect(loadSession({ fetchMe: async () => Promise.reject(e), setIdentity })).rejects.toBe(e);
    expect(setIdentity).not.toHaveBeenCalled();
  });
});
