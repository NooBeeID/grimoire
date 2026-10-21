import { describe, expect, it } from 'vitest';
import { mockAdmin, mockDimas, mockRani, mockYoga } from '../mocks/invitations';
import { userInvitationBadge } from './invitation';

describe('userInvitationBadge', () => {
  it.each([
    [mockAdmin, null],
    [mockDimas, 'pending'],
    [mockRani, 'failed'],
    [mockYoga, 'expired'],
    [{ invitation: { ...mockRani.invitation, delivery_status: 'failed' as const } }, 'failed'],
    [{ invitation: { ...mockRani.invitation, delivery_status: 'pending' as const } }, 'pending'],
    [{ invitation: { ...mockYoga.invitation, delivery_status: 'failed' as const } }, 'expired'],
  ])('%#', (u, want) => expect(userInvitationBadge(u)).toBe(want));
});
