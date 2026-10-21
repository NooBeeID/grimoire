import { describe, expect, it } from 'vitest';
import { fromDrop, fromInput, MAX_DROP_ENTRIES } from './collect';

describe('fromInput', () => {
  it('uses webkitRelativePath and falls back to name', () => {
    const a = new File(['x'], 'login.md');
    Object.defineProperty(a, 'webkitRelativePath', { value: 'docs/auth/login.md' });
    const b = new File(['x'], 'solo.md');
    const list = { 0: a, 1: b, length: 2, [Symbol.iterator]: Array.prototype[Symbol.iterator] } as unknown as FileList;
    expect(fromInput(list).map((f) => f.path)).toEqual(['docs/auth/login.md', 'solo.md']);
  });
});

// Entri palsu mengikuti API FileSystemEntry.
const fileEntry = (fullPath: string, content = 'x') =>
  ({ isFile: true, isDirectory: false, fullPath, name: fullPath.split('/').pop(), file: (ok: (f: File) => void) => ok(new File([content], fullPath.split('/').pop()!)) }) as unknown as FileSystemEntry;
function dirEntry(fullPath: string, children: FileSystemEntry[], batchSize = 2): FileSystemEntry {
  return {
    isFile: false,
    isDirectory: true,
    fullPath,
    name: fullPath.split('/').pop(),
    createReader() {
      let i = 0;
      return {
        readEntries(ok: (e: FileSystemEntry[]) => void) {
          const batch = children.slice(i, i + batchSize);
          i += batchSize;
          ok(batch);
        },
      };
    },
  } as unknown as FileSystemEntry;
}
const dt = (entries: FileSystemEntry[], files: File[] = []) =>
  ({
    items: entries.map((e) => ({ kind: 'file', webkitGetAsEntry: () => e })),
    files,
  }) as unknown as DataTransfer;

describe('fromDrop', () => {
  it('reads a three-level folder recursively, repeating readEntries until empty', async () => {
    const tree = dirEntry('/docs', [
      fileEntry('/docs/a.md'),
      dirEntry('/docs/auth', [fileEntry('/docs/auth/login.md'), fileEntry('/docs/auth/sesi.md'), dirEntry('/docs/auth/flow', [fileEntry('/docs/auth/flow/x.md')])]),
      fileEntry('/docs/b.md'),
    ]);
    const r = await fromDrop(dt([tree]));
    expect(r.truncated).toBe(false);
    expect(r.files.map((f) => f.path).sort()).toEqual(['docs/a.md', 'docs/auth/flow/x.md', 'docs/auth/login.md', 'docs/auth/sesi.md', 'docs/b.md']);
  });
  it('strips the leading slash and keeps loose dropped files at the root', async () => {
    const r = await fromDrop(dt([fileEntry('/a.md'), fileEntry('/b.md')]));
    expect(r.files.map((f) => f.path)).toEqual(['a.md', 'b.md']);
  });
  it('falls back to dt.files when the entry API is unavailable', async () => {
    const files = [new File(['x'], 'a.md'), new File(['y'], 'b.txt')];
    const noApi = { items: [{ kind: 'file' }], files } as unknown as DataTransfer;
    const r = await fromDrop(noApi);
    expect(r.files.map((f) => f.path)).toEqual(['a.md', 'b.txt']);
  });
  it('stops at the entry limit and flags truncation', async () => {
    const many = Array.from({ length: MAX_DROP_ENTRIES + 5 }, (_, i) => fileEntry(`/d/f${i}.md`));
    const r = await fromDrop(dt([dirEntry('/d', many, 500)]));
    expect(r.files).toHaveLength(MAX_DROP_ENTRIES);
    expect(r.truncated).toBe(true);
  });
});
