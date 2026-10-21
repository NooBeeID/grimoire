import { describe, expect, it } from 'vitest';
import { mockCandidate } from '../mocks/members';
import { mockInvitations } from '../mocks/invitations';
import { inviteOptionFor, invitationStatus, isEmailLike, normalizeEmail, sortInvitations } from './invite';

describe('isEmailLike', () => {
  it.each([
    ['baru@tim.id', true],
    ['baru@tim', false],
    ['a b@tim.id', false],
    ['a@@tim.id', false],
    ['@tim.id', false],
    [`${'a'.repeat(250)}@tim.id`, false],
  ])('%s -> %s', (q, ok) => expect(isEmailLike(q)).toBe(ok));
});

describe('normalizeEmail', () => {
  it('trim dan lowercase', () => expect(normalizeEmail(' Baru@Tim.ID ')).toBe('baru@tim.id'));
});

describe('inviteOptionFor', () => {
  const ready = (items = [] as ReturnType<typeof mockCandidate>[]) => ({ status: 'ready' as const, items: items as never });
  it('bukan email -> null', () => expect(inviteOptionFor('budi', ready(), [])).toBeNull());
  it('belum ready -> null', () => expect(inviteOptionFor('baru@tim.id', { status: 'loading', items: [] }, [])).toBeNull());
  it('kandidat dengan email sama persis -> null', () => {
    expect(inviteOptionFor('budi1@tim.id', ready([mockCandidate(1)]), [])).toBeNull();
  });
  it('kandidat mirip saja -> invite', () => {
    expect(inviteOptionFor('budi@tim.id', ready([mockCandidate(1)]), [])).toBe('invite');
  });
  it('nol kandidat -> invite', () => expect(inviteOptionFor('baru@tim.id', ready(), [])).toBe('invite'));
  it('email anggota (tanpa beda huruf) -> member', () => {
    expect(inviteOptionFor('Ana@Tim.id', ready(), [{ email: 'ana@tim.id' }])).toBe('member');
  });
});

describe('invitationStatus', () => {
  it.each([
    [{ expired: true, delivery_status: 'failed' as const }, 'expired'],
    [{ expired: false, delivery_status: 'failed' as const }, 'failed'],
    [{ expired: false, delivery_status: 'not_configured' as const }, 'not_configured'],
    [{ expired: false, delivery_status: 'pending' as const }, 'pending'],
    [{ expired: false, delivery_status: 'sent' as const }, 'sent'],
  ])('%j -> %s', (i, want) => expect(invitationStatus(i)).toBe(want));
});

describe('sortInvitations', () => {
  it('added_at menurun, tie-break user_id naik', () => {
    const [a, b, c] = mockInvitations as [(typeof mockInvitations)[number], (typeof mockInvitations)[number], (typeof mockInvitations)[number]];
    const tie = { ...b, user_id: '00000000-0000-4000-8000-000000000000' };
    expect(sortInvitations([c, b, a]).map((x) => x.user_id)).toEqual([a.user_id, b.user_id, c.user_id]);
    expect(sortInvitations([b, tie]).map((x) => x.user_id)).toEqual([tie.user_id, b.user_id]);
  });
});
