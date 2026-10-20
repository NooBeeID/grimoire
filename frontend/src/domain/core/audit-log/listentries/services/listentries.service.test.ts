import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockList } from '../mocks/entries';
import { EMPTY_FILTER, type AuditFilter } from '../types';
import { AuditListSchema, buildListQuery, listEntries, resolveRange } from './listentries.service';

const F = (o: Partial<AuditFilter> = {}): AuditFilter => ({ ...EMPTY_FILTER, actions: [], statuses: [], ...o });
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('buildListQuery', () => {
  it('repeats action/status, drops empties, encodes, no range', () => {
    const q = buildListQuery(
      F({ actions: ['a.b', 'c.d'], statuses: ['ok', 'error'], actorLabel: 'a&b %', target: 'x y', range: '7d' }),
      { from: '2026-10-13T00:00:00Z' },
      'CUR',
    );
    const p = new URL(q, 'http://x').searchParams;
    expect(p.getAll('action')).toEqual(['a.b', 'c.d']);
    expect(p.getAll('status')).toEqual(['ok', 'error']);
    expect(p.get('actor_label')).toBe('a&b %');
    expect(p.get('target')).toBe('x y');
    expect(p.get('cursor')).toBe('CUR');
    expect(p.get('limit')).toBe('50');
    expect(p.has('range')).toBe(false);
    expect(p.has('project_id')).toBe(false);
  });
  it('actor_id only with actor_type', () => {
    const id = '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c';
    expect(buildListQuery(F({ actorId: id }), {}, null)).not.toContain('actor_id');
    expect(buildListQuery(F({ actorId: id, actorType: 'user' }), {}, null)).toContain('actor_id=' + id);
  });
});

describe('resolveRange', () => {
  const now = new Date('2026-10-20T00:00:00Z');
  it('presets, all, custom', () => {
    expect(resolveRange(F({ range: '7d' }), now)).toEqual({ from: '2026-10-13T00:00:00Z' });
    expect(resolveRange(F({ range: 'all' }), now)).toEqual({});
    expect(resolveRange(F({ range: 'custom', from: 'A', to: 'B' }), now)).toEqual({ from: 'A', to: 'B' });
  });
});

describe('listEntries', () => {
  it('mock matches schema and maps nextCursor', async () => {
    expect(AuditListSchema.safeParse(mockList(2, 'c')).success).toBe(true);
    stub(200, mockList(2, 'c'));
    const r = await listEntries(F(), {}, null);
    expect(r.items).toHaveLength(2);
    expect(r.nextCursor).toBe('c');
  });
  it('null cursor', async () => {
    stub(200, mockList(1));
    expect((await listEntries(F(), {}, null)).nextCursor).toBeNull();
  });
  it('broken item -> INVALID_RESPONSE', async () => {
    stub(200, { items: [{ id: 'x' }], next_cursor: null });
    await expect(listEntries(F(), {}, null)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('400 keeps details.field; 403 status', async () => {
    stub(400, { error: { code: 'INVALID_ARGUMENT', message: 'm', details: { field: 'from' } }, trace_id: 't' });
    await expect(listEntries(F(), {}, null)).rejects.toMatchObject({ status: 400, details: { field: 'from' } });
    stub(403, { error: { code: 'FORBIDDEN', message: 'm' }, trace_id: 't' });
    await expect(listEntries(F(), {}, null)).rejects.toMatchObject({ status: 403 });
  });
});
