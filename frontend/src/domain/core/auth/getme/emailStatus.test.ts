import { describe, expect, it, vi } from 'vitest';
import { refreshEmailStatus } from './emailStatus';
import type { Me } from './types';

const me: Me = { id: '1', email: 'a@b.c', name: 'A', role: 'developer', must_change_password: false, email_enabled: true };

describe('refreshEmailStatus', () => {
  it('mengisi store', async () => {
    const setMailStatus = vi.fn();
    await refreshEmailStatus({ getMe: async () => me, setMailStatus });
    expect(setMailStatus).toHaveBeenCalledWith(true);
  });
  it('dua panggilan bersamaan memakai satu getMe', async () => {
    const getMe = vi.fn(async () => me);
    const setMailStatus = vi.fn();
    await Promise.all([refreshEmailStatus({ getMe, setMailStatus }), refreshEmailStatus({ getMe, setMailStatus })]);
    expect(getMe).toHaveBeenCalledTimes(1);
  });
  it('gagal -> diam dan store tidak diubah', async () => {
    const setMailStatus = vi.fn();
    await expect(refreshEmailStatus({ getMe: async () => Promise.reject(new Error('x')), setMailStatus })).resolves.toBeUndefined();
    expect(setMailStatus).not.toHaveBeenCalled();
  });
});
