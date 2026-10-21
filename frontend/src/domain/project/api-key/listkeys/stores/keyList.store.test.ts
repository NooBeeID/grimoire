import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockKey, mockRevokedKey } from '../mocks/keys';
import { activeCount, createKeyListStore } from './keyList.store';

const err = (status: number) => new ApiError({ status, code: 'X', message: 'm' });

describe('keyList store', () => {
  it('begin/resolve -> ready sorted', () => {
    const s = createKeyListStore();
    s.begin('p');
    s.resolve('p', [mockRevokedKey(2), mockKey(1)]);
    const st = get(s);
    expect(st.status === 'ready' && st.items.map((k) => k.status)).toEqual(['active', 'revoked']);
    expect(activeCount(st)).toBe(1);
  });
  it('fail 404 -> notFound; 500 keeps previous', () => {
    const s = createKeyListStore();
    s.begin('p');
    s.resolve('p', [mockKey(1)]);
    s.begin('p');
    s.fail('p', err(500));
    const st = get(s);
    expect(st.status === 'error' && st.previous).toHaveLength(1);
    s.fail('p', err(404));
    expect(get(s).status).toBe('notFound');
  });
  it('ignores stale project results', () => {
    const s = createKeyListStore();
    s.begin('p2');
    s.resolve('p1', [mockKey(1)]);
    expect(get(s).status).toBe('loading');
  });
  it('upsert inserts at top of active, revoked version moves down; only when ready', () => {
    const s = createKeyListStore();
    s.upsert(mockKey(1));
    expect(get(s).status).toBe('idle');
    s.begin('p');
    s.resolve('p', [mockKey(1, { created_at: '2026-10-23T08:00:00Z' })]);
    s.upsert(mockKey(2, { created_at: '2026-10-23T09:00:00Z' }));
    let st = get(s);
    expect(st.status === 'ready' && st.items.map((k) => k.id.slice(-1))).toEqual(['2', '1']);
    s.upsert(mockKey(2, { status: 'revoked', created_at: '2026-10-23T09:00:00Z' }));
    st = get(s);
    expect(st.status === 'ready' && st.items.map((k) => k.id.slice(-1))).toEqual(['1', '2']);
  });
  it('invalidate bumps version', () => {
    const s = createKeyListStore();
    s.invalidate();
    expect(get(s.version)).toBe(1);
  });
});
