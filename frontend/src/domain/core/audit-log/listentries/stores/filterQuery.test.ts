import { describe, expect, it } from 'vitest';
import { EMPTY_FILTER, type AuditFilter } from '../types';
import { filterFromQuery, filterKey, filterToQuery, validateFilter } from './filterQuery';

const U = '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c';
const F = (o: Partial<AuditFilter> = {}): AuditFilter => ({ ...EMPTY_FILTER, actions: [], statuses: [], ...o });

describe('filterQuery', () => {
  it('round-trips a full filter with stable order', () => {
    const f = F({
      actorType: 'api_key', actorId: U, actorLabel: 'bud', projectId: U,
      actions: ['mcp.read_file', 'auth.logout'], statuses: ['ok', 'denied'], range: '7d', target: 'x',
    });
    const q = filterToQuery(f);
    expect(filterFromQuery(q)).toEqual(f);
    expect([...q.keys()]).toEqual(['actor_type', 'actor_id', 'actor_label', 'project_id', 'action', 'action', 'status', 'status', 'range', 'target']);
  });
  it('filterToQuery drops entry, keeps unknown keys', () => {
    const out = filterToQuery(F(), new URLSearchParams('entry=1&foo=bar&status=ok'));
    expect(out.toString()).toBe('foo=bar');
  });
  it('drops wild values and caps actions', () => {
    const q = new URLSearchParams('actor_type=robot&status=x&action=BAD&project_id=abc&actor_id=zzz');
    for (let i = 0; i < 25; i++) q.append('action', `a.b${i}`);
    const f = filterFromQuery(q);
    expect(f.actorType).toBe('');
    expect(f.statuses).toEqual([]);
    expect(f.projectId).toBe('');
    expect(f.actorId).toBe('');
    expect(f.actions).toHaveLength(20);
    expect(f.actions).not.toContain('BAD');
  });
  it('from/to -> custom; range preset recognised', () => {
    expect(filterFromQuery(new URLSearchParams('from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z')).range).toBe('custom');
    expect(filterFromQuery(new URLSearchParams('range=7d')).range).toBe('7d');
    expect(filterFromQuery(new URLSearchParams('')).range).toBe('all');
  });
  it('keeps mcp.unknown_tool and mcp.tools_list through a round-trip', () => {
    const q = new URLSearchParams('action=mcp.unknown_tool&action=mcp.tools_list');
    const f = filterFromQuery(q);
    expect(f.actions).toEqual(['mcp.unknown_tool', 'mcp.tools_list']);
    expect(filterToQuery(f).toString()).toBe(q.toString());
  });
  it('filterKey ignores entry', () => {
    expect(filterKey(filterFromQuery(new URLSearchParams('action=a.b&entry=1')))).toBe(
      filterKey(filterFromQuery(new URLSearchParams('action=a.b'))),
    );
  });
  it('validateFilter', () => {
    expect(validateFilter(F({ projectId: 'x', actorId: 'y', target: 'a'.repeat(201) }))).toMatchObject({
      projectId: expect.any(String), actorId: expect.any(String), target: expect.any(String),
    });
    expect(validateFilter(F({ range: 'custom', from: '2026-10-02T00:00:00Z', to: '2026-10-01T00:00:00Z' })).range).toBeDefined();
    expect(validateFilter(F())).toEqual({});
  });
});
