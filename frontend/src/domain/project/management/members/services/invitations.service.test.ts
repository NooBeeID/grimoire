import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, isApiError } from '../../../../../shared/services/http';
import {
  mockInvitationList,
  mockInviteFallback,
  mockInviteSent,
  mockProjectId,
  mockResendFallback,
  mockResendSent,
} from '../mocks/invitations';
import { inviteToProject, listInvitations, resendProjectInvitation, revokeInvitation, toInviteError } from './invitations.service';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const call = (n = 0) => fetchMock.mock.calls[n]! as [string, RequestInit];

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('invitations.service', () => {
  it('listInvitations: GET .../invitations', async () => {
    fetchMock.mockResolvedValue(json(mockInvitationList()));
    const items = await listInvitations(mockProjectId);
    expect(items).toHaveLength(3);
    expect(call()[0]).toBe(`/api/projects/${mockProjectId}/invitations`);
  });
  it('respons dengan delivery_status asing -> INVALID_RESPONSE', async () => {
    const bad = mockInvitationList();
    fetchMock.mockResolvedValue(json({ items: [{ ...bad.items[0], delivery_status: 'queued' }] }));
    const e = await listInvitations(mockProjectId).catch((x) => x);
    expect(isApiError(e) && e.code).toBe('INVALID_RESPONSE');
  });
  it('inviteToProject: tanpa name kosong, email dinormalisasi, id di-encode', async () => {
    fetchMock.mockResolvedValue(json(mockInviteSent, 201));
    await inviteToProject('a/b', { email: ' Nadia.Putri@Tim.ID ', role: 'member', name: '  ' });
    expect(call()[0]).toBe('/api/projects/a%2Fb/invitations');
    expect(call()[1].method).toBe('POST');
    expect(JSON.parse(call()[1].body as string)).toEqual({ email: 'nadia.putri@tim.id', role: 'member' });
  });
  it('inviteToProject: name dikirim bila diisi; credentials opsional', async () => {
    fetchMock.mockResolvedValueOnce(json(mockInviteFallback, 201)).mockResolvedValueOnce(json(mockInviteSent, 201));
    const a = await inviteToProject(mockProjectId, { email: 'bagas@tim.id', role: 'owner', name: 'Bagas' });
    expect(JSON.parse(call()[1].body as string)).toEqual({ email: 'bagas@tim.id', role: 'owner', name: 'Bagas' });
    expect(a.credentials?.temporary_password).toBe('q7XkR2mVtz9HbNcP');
    const b = await inviteToProject(mockProjectId, { email: 'x@tim.id', role: 'member', name: '' });
    expect(b.credentials).toBeUndefined();
  });
  it('resend: POST tanpa body ke .../resend', async () => {
    fetchMock.mockResolvedValueOnce(json(mockResendSent)).mockResolvedValueOnce(json(mockResendFallback));
    await resendProjectInvitation(mockProjectId, 'u/1');
    expect(call()[0]).toBe(`/api/projects/${mockProjectId}/invitations/u%2F1/resend`);
    expect(call()[1].body).toBeUndefined();
    expect((await resendProjectInvitation(mockProjectId, 'u1')).credentials?.login_url).toBe('/login');
  });
  it('revoke: DELETE 204 -> void', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(revokeInvitation(mockProjectId, 'u1')).resolves.toBeUndefined();
    expect(call()[1].method).toBe('DELETE');
    expect(call()[0]).toBe(`/api/projects/${mockProjectId}/invitations/u1`);
  });
});

describe('toInviteError', () => {
  const err = (status: number, code: string, details: unknown = null) => new ApiError({ status, code, message: 'pesan server', details, traceId: 'tr1' });
  it.each([
    [err(400, 'INVALID_ARGUMENT', { fields: { email: 'format salah' } }), { kind: 'fields', fields: { email: 'format salah' } }],
    [err(409, 'CONFLICT', { reason: 'ALREADY_MEMBER' }), { kind: 'alreadyMember' }],
    [err(409, 'ALREADY_REGISTERED', { reason: 'inactive' }), { kind: 'alreadyRegistered', reason: 'inactive' }],
    [err(409, 'ALREADY_REGISTERED', { reason: 'not_developer' }), { kind: 'alreadyRegistered', reason: 'not_developer' }],
    [err(409, 'ALREADY_REGISTERED'), { kind: 'alreadyRegistered', reason: 'registered' }],
    [err(409, 'CONFLICT', { reason: 'INVITATION_NOT_PENDING' }), { kind: 'notPending' }],
    [err(409, 'CONFLICT', { reason: 'LAST_OWNER' }), { kind: 'lastOwner' }],
    [err(429, 'RATE_LIMITED', { retry_after_seconds: 1840, scope: 'resend' }), { kind: 'rateLimited', scope: 'resend', minutes: 31 }],
    [err(429, 'RATE_LIMITED'), { kind: 'rateLimited', scope: 'invite', minutes: 60 }],
    [err(403, 'FORBIDDEN'), { kind: 'forbidden' }],
    [err(404, 'NOT_FOUND'), { kind: 'notFound' }],
    [err(500, 'INTERNAL'), { kind: 'generic', message: 'pesan server', traceId: 'tr1' }],
  ])('%#', (e, want) => expect(toInviteError(e)).toEqual(want));
  it('429 tanpa details memakai scope pemanggil', () => {
    expect(toInviteError(err(429, 'RATE_LIMITED'), 'resend')).toEqual({ kind: 'rateLimited', scope: 'resend', minutes: 60 });
  });
});
