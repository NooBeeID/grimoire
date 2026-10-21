import { describe, expect, it } from 'vitest';
import { canDeleteProject, canManageMembers } from './types';

describe('access helpers', () => {
  it('owner/admin can manage and delete; member cannot', () => {
    for (const l of ['owner', 'admin'] as const) {
      expect(canManageMembers(l)).toBe(true);
      expect(canDeleteProject(l)).toBe(true);
    }
    expect(canManageMembers('member')).toBe(false);
    expect(canDeleteProject('member')).toBe(false);
  });
});
