import { describe, expect, it } from 'vitest';
import { ACTION_GROUPS, ACTION_RE, AUDIT_ACTIONS } from './types';

describe('action catalog', () => {
  it('has 40 unique valid actions, each in exactly one group', () => {
    expect(AUDIT_ACTIONS).toHaveLength(40);
    expect(new Set(AUDIT_ACTIONS).size).toBe(40);
    for (const a of AUDIT_ACTIONS) {
      expect(ACTION_RE.test(a)).toBe(true);
      expect(ACTION_GROUPS.filter((g) => (g.actions as readonly string[]).includes(a))).toHaveLength(1);
    }
  });

  it('puts the gateway events in the mcp group', () => {
    const mcp = ACTION_GROUPS.find((g) => g.key === 'mcp')?.actions as readonly string[];
    expect(mcp).toEqual(expect.arrayContaining(['mcp.initialize', 'mcp.tools_list', 'mcp.unknown_tool']));
    expect(AUDIT_ACTIONS).not.toContain('mcp.auth');
    expect(AUDIT_ACTIONS).not.toContain('mcp.tools/list');
  });
});
