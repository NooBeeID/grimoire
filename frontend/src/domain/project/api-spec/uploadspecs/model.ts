import {
  SPEC_EXTENSIONS,
  NAME_RE,
  type ApiSpecFileResult,
  type FileStatus,
  type PickedFile,
} from './types';

export type LocalResult = ApiSpecFileResult & { local: true };

const hasSpecExt = (path: string) => SPEC_EXTENSIONS.some((e) => path.toLowerCase().endsWith(e));

// File non-spec tidak dikirim; dilaporkan sebagai skipped lokal (hanya UX, backend tetap otoritatif).
export function partitionPicked(files: PickedFile[]): { spec: PickedFile[]; local: LocalResult[] } {
  const spec: PickedFile[] = [];
  const local: LocalResult[] = [];
  for (const f of files) {
    if (hasSpecExt(f.path)) spec.push(f);
    else
      local.push({
        local: true,
        file: f.file.name,
        source_path: f.path,
        name: null,
        status: 'skipped',
        reason_code: 'NOT_SPEC_FILE',
        reason: null,
        issues: [],
        openapi_version: null,
        title: null,
        api_version: null,
        endpoint_count: null,
      });
  }
  return { spec, local };
}

export function validateNameInput(name: string, specCount: number): 'ok' | 'INVALID_NAME' | 'NAME_WITH_MULTIPLE_FILES' {
  if (name === '') return 'ok';
  if (specCount !== 1) return 'NAME_WITH_MULTIPLE_FILES';
  if (!NAME_RE.test(name) || name === '.' || name === '..') return 'INVALID_NAME';
  return 'ok';
}

export function summarize(rs: ApiSpecFileResult[]): Record<FileStatus, number> {
  const out: Record<FileStatus, number> = { created: 0, updated: 0, unchanged: 0, skipped: 0, rejected: 0, valid: 0 };
  for (const r of rs) out[r.status]++;
  return out;
}

// Hasil server dulu (urutan part), lalu hasil lokal.
export function mergeResults(server: ApiSpecFileResult[], local: ApiSpecFileResult[]): ApiSpecFileResult[] {
  return [...server, ...local];
}

// Teks salin: "<source_path ?? file> [<status>/<reason_code>]: <reason>" + satu baris per issue. Tanpa isi spec.
export function issueText(r: ApiSpecFileResult): string {
  const head = `${r.source_path ?? r.file} [${r.status}${r.reason_code ? '/' + r.reason_code : ''}]${r.reason ? ': ' + r.reason : ''}`;
  return [head, ...r.issues.map((i) => `  - ${i.code} ${i.message}`)].join('\n');
}
export function rejectedText(rs: ApiSpecFileResult[]): string {
  return rs.filter((r) => r.status === 'rejected').map(issueText).join('\n');
}

export function markOverwrite(rs: ApiSpecFileResult[], existing: Set<string>): Array<ApiSpecFileResult & { willOverwrite: boolean }> {
  return rs.map((r) => ({ ...r, willOverwrite: r.status === 'valid' && r.name !== null && existing.has(r.name) }));
}
