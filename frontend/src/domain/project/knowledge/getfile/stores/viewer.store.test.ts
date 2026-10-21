import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockFile } from '../../base';
import { createViewerStore } from './viewer.store';

describe('viewer store', () => {
  it('ignores a stale response for an older path', () => {
    const s = createViewerStore();
    s.begin('a.md');
    s.begin('b.md');
    s.resolve(mockFile('a.md'));
    expect(get(s).status).toBe('loading');
    s.resolve(mockFile('b.md'));
    expect(get(s).status).toBe('ready');
  });
  it('404 -> notFound, others -> error', () => {
    const s = createViewerStore();
    s.begin('a.md');
    s.fail('a.md', new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    expect(get(s).status).toBe('notFound');
    s.begin('a.md');
    s.fail('a.md', new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    expect(get(s).status).toBe('error');
  });
});
