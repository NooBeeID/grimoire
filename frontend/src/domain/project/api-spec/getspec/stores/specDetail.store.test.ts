import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mockDetail } from '../mocks/detail';
import { createSpecDetailStore } from './specDetail.store';

describe('specDetail store', () => {
  it('set derives the tag tree; setQuery filters matches', () => {
    const s = createSpecDetailStore();
    s.set(mockDetail());
    expect(get(s.tree).length).toBe(3);
    s.setQuery('refund');
    expect(get(s.matches)).toHaveLength(1);
  });
  it('replaceSpec swaps metadata only', () => {
    const s = createSpecDetailStore();
    const d = mockDetail();
    s.set(d);
    s.replaceSpec({ ...d.spec, name: 'billing' });
    const v = get(s);
    expect(v.status === 'ready' && v.detail.spec.name).toBe('billing');
    expect(v.status === 'ready' && v.detail.document).toBe(d.document);
  });
});
