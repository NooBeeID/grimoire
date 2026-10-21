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

describe('candidateSearch store: opsi undang', () => {
  it('move memasukkan opsi undang sebagai indeks terakhir hanya bila inviteOption invite', () => {
    const st = createCandidateSearchStore();
    st.setQuery('baru@tim.id');
    st.begin(1);
    st.resolve(1, []);
    st.move(1);
    expect(get(st).activeIndex).toBe(-1);
    st.setInviteOption('invite');
    st.move(1);
    expect(get(st).activeIndex).toBe(0);
    st.move(1);
    expect(get(st).activeIndex).toBe(0);
  });
  it('setQuery me-reset inviteOption', () => {
    const st = createCandidateSearchStore();
    st.setInviteOption('member');
    st.setQuery('x');
    expect(get(st).inviteOption).toBeNull();
  });
});
