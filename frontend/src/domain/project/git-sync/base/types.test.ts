import { describe, expect, it } from 'vitest';
import { mockApiSpecReport, mockConfig, mockKnowledgeReport } from './mocks';
import { GitSyncConfigSchema, GitSyncReportItemSchema, GitSyncReportSchema } from './types';

describe('git-sync base schemas', () => {
  it('accepts the knowledge and api_spec reports', () => {
    expect(GitSyncReportSchema.safeParse(mockKnowledgeReport()).success).toBe(true);
    expect(GitSyncReportSchema.safeParse(mockApiSpecReport()).success).toBe(true);
  });
  it('accepts a config with last_result null', () => {
    expect(GitSyncConfigSchema.safeParse(mockConfig({ last_result: null })).success).toBe(true);
  });
  it('accepts a config that never synced', () => {
    expect(GitSyncConfigSchema.safeParse(mockConfig({ last_status: null, last_synced_at: null, last_synced_by: null, last_result: null })).success).toBe(true);
  });
  it('rejects an unknown item status', () => {
    const bad = { ...mockKnowledgeReport().items[0], status: 'exploded' };
    expect(GitSyncReportItemSchema.safeParse(bad).success).toBe(false);
  });
  it('api_spec fields are optional', () => {
    const item = mockKnowledgeReport().items[0]!;
    expect(GitSyncReportItemSchema.safeParse(item).success).toBe(true);
    expect(GitSyncReportItemSchema.safeParse({ ...item, endpoint_count: 14, title: 'Billing API', openapi_version: '3.0.3' }).success).toBe(true);
  });
});
