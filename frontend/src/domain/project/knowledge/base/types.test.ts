import { describe, expect, it } from 'vitest';
import { ConflictDetailsSchema, KnowledgeFileSchema } from './types';
import { mockFile, V2 } from './mocks';

describe('knowledge base schemas', () => {
  it('accepts a valid file', () => expect(KnowledgeFileSchema.safeParse(mockFile('a.md')).success).toBe(true));
  it('rejects uppercase or short version', () => {
    expect(KnowledgeFileSchema.safeParse(mockFile('a.md', { version: 'A'.repeat(64) })).success).toBe(false);
    expect(KnowledgeFileSchema.safeParse(mockFile('a.md', { version: 'a'.repeat(63) })).success).toBe(false);
  });
  it('accepts conflict details', () => {
    const d = { reason: 'version_conflict', current: { content: 'x', version: V2, updated_at: '2026-10-21T09:30:00Z', updated_by_label: 'budi@tim.id' } };
    expect(ConflictDetailsSchema.safeParse(d).success).toBe(true);
  });
});
