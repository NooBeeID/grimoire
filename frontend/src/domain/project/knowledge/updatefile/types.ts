import type { ConflictCurrent, KnowledgeFile } from '../base';

export interface UpdateFileInput {
  content: string;
  version: string;
}
export type SaveOutcome =
  | { kind: 'saved'; file: KnowledgeFile }
  | { kind: 'conflict'; current: ConflictCurrent }
  | { kind: 'notFound' }
  | { kind: 'failed'; error: unknown };

export interface Draft {
  content: string;
  baseVersion: string;
  savedAt: string;
}
