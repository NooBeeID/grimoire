import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { performLogout } from './logout';

const err = (status: number) => new ApiError({ status, code: 'X', message: 'm' });

describe('performLogout', () => {
  it('ok on success', async () => {
    expect(await performLogout({ logoutRequest: async () => {} })).toEqual({ ok: true });
  });
  it('401 counts as success', async () => {
    expect(await performLogout({ logoutRequest: async () => Promise.reject(err(401)) })).toEqual({ ok: true });
  });
  it('other errors are returned', async () => {
    const e = err(500);
    expect(await performLogout({ logoutRequest: async () => Promise.reject(e) })).toEqual({ ok: false, error: e });
  });
});
