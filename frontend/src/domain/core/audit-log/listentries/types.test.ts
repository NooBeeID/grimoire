import { describe, expect, it } from 'vitest';
import { ACTION_GROUPS, ACTION_RE, AUDIT_ACTIONS } from './types';

describe('action catalog', () => {
  it('has 33 unique valid actions, each in exactly one group', () => {
    expect(AUDIT_ACTIONS).toHaveLength(33);
    expect(new Set(AUDIT_ACTIONS).size).toBe(33);
    for (const a of AUDIT_ACTIONS) {
      expect(ACTION_RE.test(a)).toBe(true);
      expect(ACTION_GROUPS.filter((g) => (g.actions as readonly string[]).includes(a))).toHaveLength(1);
    }
  });
});
