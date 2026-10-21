import { describe, expect, it } from 'vitest';
import { mockEndpoints } from '../mocks/detail-3.0';
import type { ApiSpecEndpoint } from '../types';
import { filterEndpoints } from './search';

describe('filterEndpoints', () => {
  it('returns all for an empty query', () => {
    expect(filterEndpoints(mockEndpoints, '')).toHaveLength(mockEndpoints.length);
    expect(filterEndpoints(mockEndpoints, '   ')).toHaveLength(mockEndpoints.length);
  });
  it('matches path, method, operationId, summary and tag; case-insensitive', () => {
    expect(filterEndpoints(mockEndpoints, '/health')).toHaveLength(1);
    expect(filterEndpoints(mockEndpoints, 'delete')).toHaveLength(0);
    expect(filterEndpoints(mockEndpoints, 'LISTPAYMENTS')).toHaveLength(1);
    expect(filterEndpoints(mockEndpoints, 'kembalikan')).toHaveLength(1);
    expect(filterEndpoints(mockEndpoints, 'refunds')).toHaveLength(1);
  });
  it('requires all tokens (AND)', () => {
    expect(filterEndpoints(mockEndpoints, 'POST refund').map((e) => e.operation_id)).toEqual(['refundPayment']);
    expect(filterEndpoints(mockEndpoints, 'tidak-ada')).toHaveLength(0);
  });
  it('handles 500 endpoints quickly', () => {
    const many: ApiSpecEndpoint[] = Array.from({ length: 500 }, (_, i) => ({ method: 'GET', path: `/r${i}/{id}`, operation_id: `op${i}`, summary: `s${i}`, tags: ['t'], deprecated: false }));
    const t0 = performance.now();
    expect(filterEndpoints(many, 'GET r42')).not.toHaveLength(0);
    expect(performance.now() - t0).toBeLessThan(50);
  });
});
