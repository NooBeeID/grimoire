import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockProjectList } from '../mocks/projects';
import { createProjectListStore } from './projectList.store';

const err = () => new ApiError({ status: 500, code: 'INTERNAL', message: 'x' });

describe('projectList store', () => {
  it('ignores stale resolve/fail', () => {
    const s = createProjectListStore();
    s.begin('a');
    s.begin('b');
    s.resolve('a', mockProjectList(1));
    expect(get(s).status).toBe('loading');
    s.fail('a', err());
    expect(get(s).status).toBe('loading');
    s.resolve('b', mockProjectList(1));
    expect(get(s).status).toBe('ready');
  });
  it('begin keeps previous data', () => {
    const s = createProjectListStore();
    s.begin('a');
    const d = mockProjectList(2);
    s.resolve('a', d);
    s.begin('b');
    const st = get(s);
    expect(st.status === 'loading' && st.previous).toEqual(d);
  });
  it('fail moves to error', () => {
    const s = createProjectListStore();
    s.begin('a');
    s.fail('a', err());
    expect(get(s).status).toBe('error');
  });
});
