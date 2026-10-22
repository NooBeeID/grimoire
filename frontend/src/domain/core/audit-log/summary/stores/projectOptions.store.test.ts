import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockProjectOptions } from '../mocks/summary';
import { createProjectOptionsStore } from './projectOptions.store';

describe('projectOptions store', () => {
  it('error then retry becomes ready', async () => {
    let fail = true;
    const s = createProjectOptionsStore(async () => {
      if (fail) throw new ApiError({ status: 500, code: 'INTERNAL', message: 'x' });
      return { options: mockProjectOptions.items, total: 5 };
    });
    await s.load();
    expect(get(s).status).toBe('error');
    fail = false;
    await s.retry();
    expect(get(s)).toMatchObject({ status: 'ready', total: 5 });
  });
});
