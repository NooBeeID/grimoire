import { describe, expect, it } from 'vitest';
import { issueText, markOverwrite, mergeResults, partitionPicked, rejectedText, summarize, validateNameInput } from './model';
import { mockFileResult, mockRejected } from './mocks/results';

const pf = (path: string) => ({ file: new File(['x'], path.split('/').pop()!), path });

describe('partitionPicked', () => {
  it('sends spec extensions (case-insensitive) and reports others as local skipped', () => {
    const { spec, local } = partitionPicked(['a.YAML', 'b.yml', 'specs/c.json', 'README.md', 'x.yaml.bak', 'Makefile'].map(pf));
    expect(spec.map((f) => f.path)).toEqual(['a.YAML', 'b.yml', 'specs/c.json']);
    expect(local.map((r) => r.source_path)).toEqual(['README.md', 'x.yaml.bak', 'Makefile']);
    expect(local[0]).toMatchObject({ status: 'skipped', reason_code: 'NOT_SPEC_FILE', local: true });
  });
});
describe('validateNameInput', () => {
  it('covers ok / invalid / multiple', () => {
    expect(validateNameInput('', 3)).toBe('ok');
    expect(validateNameInput('payment-v2', 1)).toBe('ok');
    for (const bad of ['Bad Name', 'a/b', '..', '.' + 'x'.repeat(80)]) expect(validateNameInput(bad, 1)).toBe('INVALID_NAME');
    expect(validateNameInput('payment', 2)).toBe('NAME_WITH_MULTIPLE_FILES');
  });
});
describe('results helpers', () => {
  it('summarize and mergeResults keep server first', () => {
    const merged = mergeResults([mockFileResult()], [mockFileResult({ status: 'skipped' })]);
    expect(merged.map((r) => r.status)).toEqual(['created', 'skipped']);
    expect(summarize(merged)).toMatchObject({ created: 1, skipped: 1, rejected: 0 });
  });
  it('issueText includes file, status, reason code and each issue', () => {
    expect(issueText(mockRejected())).toBe(
      'legacy.yaml [rejected/INVALID_SPEC]: Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1\n  - NOT_OPENAPI_3 Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)',
    );
    expect(rejectedText([mockFileResult(), mockRejected()])).toContain('legacy.yaml');
    expect(rejectedText([mockFileResult()])).toBe('');
  });
  it('markOverwrite flags only valid results with existing names', () => {
    const r = markOverwrite([mockFileResult({ status: 'valid' }), mockFileResult({ status: 'created' }), mockFileResult({ status: 'valid', name: 'auth' })], new Set(['payment']));
    expect(r.map((x) => x.willOverwrite)).toEqual([true, false, false]);
  });
});
