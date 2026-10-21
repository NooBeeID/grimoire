import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockInvitations } from '../mocks/invitations';
import { createInvitationsListStore } from './invitationsList.store';

describe('invitationsList store', () => {
  it('begin lalu resolve mengurutkan dan menandai ready', () => {
    const s = createInvitationsListStore();
    s.begin();
    expect(get(s).status).toBe('loading');
    s.resolve([...mockInvitations].reverse());
    expect(get(s).status).toBe('ready');
    expect(get(s).items.map((i) => i.email)).toEqual(['bagas@tim.id', 'nadia.putri@tim.id', 'yoga.saputra@tim.id']);
  });
  it('refresh mempertahankan item lama dengan refreshing=true', () => {
    const s = createInvitationsListStore();
    s.resolve(mockInvitations);
    s.begin();
    s.refresh();
    expect(get(s).status).toBe('ready');
    expect(get(s).items).toHaveLength(3);
    expect(get(s).refreshing).toBe(true);
  });
  it('remove, fail, clear', () => {
    const s = createInvitationsListStore();
    s.resolve(mockInvitations);
    s.remove(mockInvitations[0]!.user_id);
    expect(get(s).items).toHaveLength(2);
    s.fail(new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    expect(get(s).status).toBe('error');
    s.clear();
    expect(get(s).status).toBe('idle');
  });
});
