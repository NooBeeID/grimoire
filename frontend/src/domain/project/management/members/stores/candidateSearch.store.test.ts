import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mockCandidate } from '../mocks/members';
import { createCandidateSearchStore } from './candidateSearch.store';

describe('candidateSearch store', () => {
  it('ignores stale seq', () => {
    const s = createCandidateSearchStore();
    s.setQuery('b');
    s.begin(1);
    s.begin(2);
    s.resolve(1, [mockCandidate(1)]);
    expect(get(s).status).toBe('loading');
    s.resolve(2, [mockCandidate(2)]);
    expect(get(s).items[0]!.name).toBe('Budi 2');
    s.begin(3);
    s.fail(2, { status: 500 } as never);
    expect(get(s).status).toBe('loading');
  });
  it('move wraps at both ends', () => {
    const s = createCandidateSearchStore();
    s.setQuery('b');
    s.begin(1);
    s.resolve(1, [mockCandidate(1), mockCandidate(2)]);
    s.move(1);
    expect(get(s).activeIndex).toBe(0);
    s.move(1);
    s.move(1);
    expect(get(s).activeIndex).toBe(0);
    s.move(-1);
    expect(get(s).activeIndex).toBe(1);
  });
  it('blank query is idle; clear invalidates pending', () => {
    const s = createCandidateSearchStore();
    s.setQuery('  ');
    expect(get(s).status).toBe('idle');
    s.setQuery('a');
    s.begin(1);
    s.clear();
    s.resolve(1, [mockCandidate(1)]);
    expect(get(s).items).toEqual([]);
  });
});
