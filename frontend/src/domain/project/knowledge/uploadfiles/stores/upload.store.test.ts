import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { createUploadStore } from './upload.store';

const r = (status: 'created' | 'updated' | 'rejected' | 'unsupported' | 'skipped' | 'unchanged', path = status) => ({ path, status, target: 'knowledge' });

describe('upload store', () => {
  it('counts and filters (failed = rejected + unsupported)', () => {
    const s = createUploadStore();
    s.setPlan({ byTarget: new Map(), skipped: [r('skipped')], unsupported: [r('unsupported')], totalBytes: 0 });
    s.start();
    s.addReport([r('created'), r('updated'), r('rejected')]);
    expect(get(s.counts)).toMatchObject({ created: 1, updated: 1, rejected: 1, unsupported: 1, skipped: 1 });
    s.setFilter('failed');
    expect(get(s.filtered).map((x) => x.status).sort()).toEqual(['rejected', 'unsupported']);
    s.setFilter('updated');
    expect(get(s.filtered)).toHaveLength(1);
  });
  it('phases', () => {
    const s = createUploadStore();
    expect(get(s).phase).toBe('pick');
    s.setPlan({ byTarget: new Map(), skipped: [], unsupported: [], totalBytes: 0 });
    expect(get(s).phase).toBe('review');
    s.start();
    expect(get(s).phase).toBe('uploading');
    s.finish(true);
    expect(get(s).phase).toBe('done');
  });
});
