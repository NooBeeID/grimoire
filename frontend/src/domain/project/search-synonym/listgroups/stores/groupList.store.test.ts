import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockGroup } from '../mocks/groups';
import { createGroupListStore } from './groupList.store';

describe('groupList store', () => {
  it('begin -> resolve sorts by created_at', () => {
    const s = createGroupListStore();
    s.begin('p1');
    expect(get(s).status).toBe('loading');
    s.resolve('p1', [mockGroup(2, { created_at: '2026-10-24T00:00:00Z' }), mockGroup(1)]);
    const st = get(s);
    expect(st.status === 'ready' && st.items.map((g) => g.id.slice(-1))).toEqual(['1', '2']);
  });
  it('ignores stale resolve for another project', () => {
    const s = createGroupListStore();
    s.begin('p1');
    s.begin('p2');
    s.resolve('p1', [mockGroup(1)]);
    expect(get(s).status).toBe('loading');
  });
  it('upsert replaces by id and remove drops', () => {
    const s = createGroupListStore();
    s.begin('p1');
    s.resolve('p1', [mockGroup(1)]);
    s.upsert(mockGroup(1, { terms: ['a', 'b'] }));
    s.upsert(mockGroup(2, { created_at: '2026-10-25T00:00:00Z' }));
    let st = get(s);
    expect(st.status === 'ready' && st.items).toHaveLength(2);
    expect(st.status === 'ready' && st.items[0]!.terms).toEqual(['a', 'b']);
    s.remove(mockGroup(1).id);
    st = get(s);
    expect(st.status === 'ready' && st.items).toHaveLength(1);
  });
  it('fail 404 -> notFound, other -> error keeping previous', () => {
    const s = createGroupListStore();
    s.begin('p1');
    s.resolve('p1', [mockGroup(1)]);
    s.begin('p1');
    s.fail('p1', new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    const st = get(s);
    expect(st.status === 'error' && st.previous).toHaveLength(1);
    s.fail('p1', new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    expect(get(s).status).toBe('notFound');
  });
  it('invalidate bumps version', () => {
    const s = createGroupListStore();
    s.invalidate();
    expect(get(s.version)).toBe(1);
  });
});
