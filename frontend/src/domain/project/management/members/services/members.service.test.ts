import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { memberId, mockCandidate, mockCandidateList, mockMember, mockMemberList } from '../mocks/members';
import { addMember, changeMemberRole, listMembers, memberErrorKey, removeMember, searchCandidates } from './members.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(body === null ? null : JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const hdr = (f: ReturnType<typeof stub>) => f.mock.calls[0]![1]!.headers as Record<string, string>;
const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';

describe('members.service', () => {
  it('listMembers returns items, validates', async () => {
    const f = stub(200, mockMemberList(mockMember(1, { role: 'owner' })));
    const r = await listMembers(PID);
    expect(f.mock.calls[0]![0]).toBe(`/api/projects/${PID}/members`);
    expect(r[0]!.role).toBe('owner');
  });
  it('rejects role admin', async () => {
    stub(200, mockMemberList(mockMember(1, { role: 'admin' })));
    await expect(listMembers(PID)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('searchCandidates blank q does not fetch', async () => {
    const f = stub(200, mockCandidateList());
    await expect(searchCandidates(PID, '  ')).resolves.toEqual([]);
    expect(f).not.toHaveBeenCalled();
  });
  it('searchCandidates builds query', async () => {
    const f = stub(200, mockCandidateList(mockCandidate(1)));
    const r = await searchCandidates(PID, ' bud ');
    expect(f.mock.calls[0]![0]).toBe(`/api/projects/${PID}/member-candidates?q=bud&limit=10`);
    expect(r).toHaveLength(1);
  });
  it('addMember POST body + CSRF', async () => {
    const f = stub(201, mockMember(2));
    await addMember(PID, memberId(2), 'owner');
    expect(f.mock.calls[0]![1]!.method).toBe('POST');
    expect(JSON.parse(f.mock.calls[0]![1]!.body as string)).toEqual({ user_id: memberId(2), role: 'owner' });
    expect(hdr(f)['X-Requested-With']).toBe('grimoire');
  });
  it('removeMember DELETE without body', async () => {
    const f = stub(204, null);
    await expect(removeMember(PID, memberId(2))).resolves.toBeUndefined();
    expect(f.mock.calls[0]![1]!.method).toBe('DELETE');
    expect(f.mock.calls[0]![1]!.body).toBeUndefined();
    expect(hdr(f)['X-Requested-With']).toBe('grimoire');
  });
  it('changeMemberRole PATCH {role}', async () => {
    const f = stub(200, mockMember(2, { role: 'owner' }));
    await changeMemberRole(PID, memberId(2), 'owner');
    expect(f.mock.calls[0]![1]!.method).toBe('PATCH');
    expect(JSON.parse(f.mock.calls[0]![1]!.body as string)).toEqual({ role: 'owner' });
    expect(hdr(f)['X-Requested-With']).toBe('grimoire');
  });
});

describe('memberErrorKey', () => {
  const e = (status: number, details: unknown = null) => new ApiError({ status, code: 'X', message: 'm', details });
  it('maps known reasons, 403, 404 and unknown', () => {
    expect(memberErrorKey(e(409, { reason: 'LAST_OWNER' }))).toBe('project.error.LAST_OWNER');
    expect(memberErrorKey(e(400, { reason: 'WHATEVER' }))).toBeNull();
    expect(memberErrorKey(e(403))).toBe('error.forbidden');
    expect(memberErrorKey(e(404))).toBe('project.members.error.gone');
    expect(memberErrorKey(e(500))).toBeNull();
  });
});
