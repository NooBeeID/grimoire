import { describe, expect, it } from 'vitest';
import {
  mockProjectOptions,
  mockSummary24hProject,
  mockSummary7d,
  mockSummaryEmpty,
  mockSummaryQualityUnavailable,
} from './mocks/summary';
import { ProjectOptionListSchema, UsageSummarySchema } from './types';

describe('UsageSummarySchema', () => {
  it('accepts every mock', () => {
    for (const m of [mockSummary7d, mockSummary24hProject, mockSummaryQualityUnavailable, mockSummaryEmpty]) {
      expect(UsageSummarySchema.safeParse(m).success).toBe(true);
    }
  });
  it('mock totals are consistent', () => {
    expect(mockSummary7d.total_calls).toBe(1234);
    expect(mockSummary7d.total_errors).toBe(31);
    expect(mockSummary7d.traffic.reduce((s, p) => s + p.calls, 0)).toBe(1234);
    expect(mockSummary7d.top_projects.reduce((s, p) => s + p.calls, 0)).toBe(1234);
  });
  it('rejects unknown root key (args) and unknown tool_metrics key (query)', () => {
    expect(UsageSummarySchema.safeParse({ ...mockSummary7d, args: {} }).success).toBe(false);
    const bad = { ...mockSummary7d, tool_metrics: [{ ...mockSummary7d.tool_metrics[0], query: 'x' }] };
    expect(UsageSummarySchema.safeParse(bad).success).toBe(false);
  });
  it('accepts null latency, rejects unavailable without error_code and bucket week', () => {
    expect(mockSummary7d.tool_metrics.some((m) => m.p50_ms === null)).toBe(true);
    expect(UsageSummarySchema.safeParse({ ...mockSummary7d, search_quality: { available: false } }).success).toBe(false);
    expect(UsageSummarySchema.safeParse({ ...mockSummary7d, bucket: 'week' }).success).toBe(false);
  });
  it('project options tolerate extra fields', () => {
    expect(ProjectOptionListSchema.safeParse(mockProjectOptions).success).toBe(true);
    const extra = { items: [{ ...mockProjectOptions.items[0], my_role: 'owner' }], total: 1 };
    expect(ProjectOptionListSchema.safeParse(extra).success).toBe(true);
  });
});
