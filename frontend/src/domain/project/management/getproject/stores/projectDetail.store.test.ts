import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockDetail } from '../mocks/project';
import { createProjectDetailStore } from './projectDetail.store';

const D = mockDetail();
describe('projectDetail store', () => {
  it('404 -> notFound, other -> error', () => {
    const s = createProjectDetailStore();
    s.begin(D.id);
    s.fail(D.id, new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    expect(get(s).status).toBe('notFound');
    s.begin(D.id);
    s.fail(D.id, new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    expect(get(s).status).toBe('error');
  });
  it('replace ignores a different id and applies same id', () => {
    const s = createProjectDetailStore();
    s.begin(D.id);
    s.resolve(D.id, D);
    s.replace({ ...D, id: 'other', name: 'Z' });
    expect(get(s)).toMatchObject({ status: 'ready', project: { name: 'Payments' } });
    s.replace({ ...D, name: 'New' });
    expect(get(s)).toMatchObject({ project: { name: 'New' } });
  });
  it('begin on same id keeps previous; stale resolve ignored', () => {
    const s = createProjectDetailStore();
    s.begin(D.id);
    s.resolve(D.id, D);
    s.begin(D.id);
    expect(get(s)).toMatchObject({ status: 'loading', previous: { name: 'Payments' } });
    s.resolve('zzz', D);
    expect(get(s).status).toBe('loading');
  });
});
