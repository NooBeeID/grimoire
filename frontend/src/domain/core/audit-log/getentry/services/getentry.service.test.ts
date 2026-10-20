import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockDetail } from '../mocks/entries';
import { AuditEntryDetailSchema, getAuditEntry, toDetailError } from './getentry.service';

function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('getentry.service', () => {
  it('mock matches schema', () => {
    expect(AuditEntryDetailSchema.safeParse(mockDetail).success).toBe(true);
  });
  it('encodes id and returns parsed entry', async () => {
    const f = stub(200, mockDetail);
    const e = await getAuditEntry('a/b');
    expect(f.mock.calls[0]![0]).toBe('/api/audit-log/a%2Fb');
    expect(e.action).toBe('mcp.read_file');
  });
  it.each([
    { actor_type: 'robot' },
    { occurred_at: 'kemarin' },
    { args: [] },
  ])('rejects invalid shape %o', async (over) => {
    stub(200, { ...mockDetail, ...over });
    await expect(getAuditEntry('x')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('maps 404 envelope to ApiError', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'nf' }, trace_id: 't' });
    await expect(getAuditEntry('x')).rejects.toMatchObject({ status: 404, code: 'NOT_FOUND' });
  });
  it('toDetailError', () => {
    const mk = (status: number) => new ApiError({ status, code: 'X', message: 'm' });
    expect(toDetailError(mk(404))).toBe('notFound');
    expect(toDetailError(mk(403))).toBe('forbidden');
    expect(toDetailError(mk(500))).toBe('load');
    expect(toDetailError(mk(0))).toBe('load');
  });
});
