import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockDetail } from '../mocks/entries';
import { createEntryDetailStore } from './entryDetail.store';

describe('entryDetail.store', () => {
  it('ignores stale resolve and maps errors', () => {
    const s = createEntryDetailStore();
    s.begin('A');
    expect(get(s)).toEqual({ status: 'loading', id: 'A' });
    s.resolve('B', mockDetail);
    expect(get(s).status).toBe('loading');
    s.fail('A', new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    expect(get(s)).toMatchObject({ status: 'error', error: 'notFound' });
    s.reset();
    expect(get(s)).toEqual({ status: 'idle' });
  });
});
