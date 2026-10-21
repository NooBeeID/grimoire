import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockAuthSpec, mockSpec } from '../mocks/specs';
import { createSpecListStore } from './specList.store';

function ready() {
  const s = createSpecListStore();
  s.begin('p');
  s.resolve('p', [mockSpec()], 1, 100);
  return s;
}
const names = (s: ReturnType<typeof createSpecListStore>) => {
  const v = get(s);
  return v.status === 'ready' ? v.items.map((x) => x.name) : [];
};

describe('specList store', () => {
  it('upsert new spec keeps name order and raises total; same name replaces', () => {
    const s = ready();
    s.upsert(mockAuthSpec());
    expect(names(s)).toEqual(['auth', 'payment']);
    expect((get(s) as { total: number }).total).toBe(2);
    s.upsert(mockAuthSpec({ title: 'Auth v2' }));
    expect((get(s) as { total: number }).total).toBe(2);
  });
  it('rename moves position', () => {
    const s = ready();
    s.rename('payment', mockSpec({ name: 'billing' }));
    expect(names(s)).toEqual(['billing']);
  });
  it('remove lowers total; invalidate bumps version; names()', () => {
    const s = ready();
    expect(s.names().has('payment')).toBe(true);
    s.remove('payment');
    expect(names(s)).toEqual([]);
    expect((get(s) as { total: number }).total).toBe(0);
    const v = get(s.version);
    s.invalidate();
    expect(get(s.version)).toBe(v + 1);
  });
  it('ignores stale resolve and maps 404 to notFound', () => {
    const s = createSpecListStore();
    s.begin('a');
    s.resolve('b', [], 0, 1);
    expect(get(s).status).toBe('loading');
    s.fail('a', new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    expect(get(s).status).toBe('notFound');
  });
  it('keeps previous items while reloading', () => {
    const s = ready();
    s.begin('p');
    const v = get(s);
    expect(v.status === 'loading' && v.previous).toHaveLength(1);
  });
});
