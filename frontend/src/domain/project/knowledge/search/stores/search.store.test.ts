import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { createSearchStore } from './search.store';

describe('search store', () => {
  it('ignores stale results and keeps previous while loading', () => {
    const s = createSearchStore();
    s.begin('a|0|20');
    s.resolve('a|0|20', { items: [], total: 0 });
    s.begin('b|0|20');
    s.resolve('a|0|20', { items: [], total: 9 });
    const st = get(s);
    expect(st.status).toBe('loading');
    expect(st.status === 'loading' && st.previous?.total).toBe(0);
  });
});
