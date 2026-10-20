import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockSummary } from '../mocks/entries';
import { createListEntriesStore } from './listentries.store';

describe('listentries.store', () => {
  it('pages, dedupe, nextCursor, failMore, reset', () => {
    const s = createListEntriesStore();
    s.reset('k', { from: 'F' });
    s.begin();
    s.pushPage({ cursor: null, items: [mockSummary(1), mockSummary(2)], nextCursor: 'c1' });
    s.pushPage({ cursor: 'c1', items: [mockSummary(2), mockSummary(3)], nextCursor: null });
    expect(get(s.rows).map((r) => r.id.slice(-1))).toEqual(['1', '2', '3']);
    expect(get(s.nextCursor)).toBeNull();
    expect(get(s.hasMore)).toBe(false);
    s.failMore(new ApiError({ status: 500, code: 'X', message: 'm' }));
    expect(get(s).moreStatus).toBe('error');
    expect(get(s.rows)).toHaveLength(3);
    expect(get(s).range).toEqual({ from: 'F' });
    s.reset('k2', {});
    expect(get(s.rows)).toHaveLength(0);
  });
  it('suggestions distinct, cap 50, skip null project', () => {
    const s = createListEntriesStore();
    s.reset('k', {});
    const items = Array.from({ length: 60 }, (_, i) =>
      mockSummary(i + 1, { actor_label: `a${i}`, actor_id: null, project_id: i % 2 ? null : `p${i}`, project_label: 'P' }),
    );
    s.pushPage({ cursor: null, items, nextCursor: null });
    const sg = get(s.suggestions);
    expect(sg.actors).toHaveLength(50);
    expect(sg.projects.every((p) => p.projectId)).toBe(true);
    expect(sg.projects).toHaveLength(30);
  });
});
