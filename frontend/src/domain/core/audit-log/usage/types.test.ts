import { describe, expect, it } from 'vitest';
import { mockUsageAll, mockUsageNoProject } from './mocks/usage';
import { AgentUsageSchema } from './types';

describe('AgentUsageSchema', () => {
  it('parses full mock', () => {
    expect(AgentUsageSchema.parse(mockUsageAll).per_key?.items).toHaveLength(2);
    expect(AgentUsageSchema.parse(mockUsageNoProject).top_files?.error_code).toBe('PROJECT_REQUIRED');
  });
  it('accepts null sections', () => {
    const only = { ...mockUsageAll, top_files: null, unread_files: null, zero_queries: null };
    expect(AgentUsageSchema.parse(only).per_key).not.toBeNull();
  });
  it('rejects extra field in item (anti-leak)', () => {
    const bad = structuredClone(mockUsageAll) as unknown as { top_files: { items: Record<string, unknown>[] } };
    bad.top_files.items[0]!.snippet = 'isi file';
    expect(() => AgentUsageSchema.parse(bad)).toThrow();
  });
  it('rejects unknown error_code', () => {
    const bad = { ...mockUsageAll, zero_queries: { available: false, error_code: 'BOOM', items: [] } };
    expect(() => AgentUsageSchema.parse(bad)).toThrow();
  });
});
