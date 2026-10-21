import type { LocalFile } from '../types';

export function fromInput(list: FileList): LocalFile[] {
  return Array.from(list).map((file) => ({ path: file.webkitRelativePath || file.name, file }));
}

export const MAX_DROP_ENTRIES = 10_000;

export interface DropResult {
  files: LocalFile[];
  truncated: boolean;
}

// Pembacaan rekursif folder yang di-drop lewat webkitGetAsEntry; readEntries mengembalikan batch
// (Chrome 100) sehingga harus diulang sampai kosong. Tanpa API entri: jatuh ke dt.files (tanpa folder).
export async function fromDrop(dt: DataTransfer): Promise<DropResult> {
  const out: LocalFile[] = [];
  let truncated = false;
  const roots: FileSystemEntry[] = [];
  for (const item of Array.from(dt.items ?? [])) {
    const entry = item.kind === 'file' && typeof item.webkitGetAsEntry === 'function' ? item.webkitGetAsEntry() : null;
    if (entry) roots.push(entry);
  }
  if (roots.length === 0) {
    for (const file of Array.from(dt.files ?? [])) out.push({ path: file.name, file });
    return { files: out.slice(0, MAX_DROP_ENTRIES), truncated: out.length > MAX_DROP_ENTRIES };
  }

  const readBatch = (reader: FileSystemDirectoryReader) =>
    new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
  const fileOf = (e: FileSystemFileEntry) => new Promise<File>((resolve, reject) => e.file(resolve, reject));

  async function walk(entry: FileSystemEntry): Promise<void> {
    if (truncated) return;
    if (entry.isFile) {
      if (out.length >= MAX_DROP_ENTRIES) {
        truncated = true;
        return;
      }
      out.push({ path: entry.fullPath.replace(/^\/+/, ''), file: await fileOf(entry as FileSystemFileEntry) });
      return;
    }
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    for (;;) {
      const batch = await readBatch(reader);
      if (batch.length === 0) break;
      for (const child of batch) await walk(child);
      if (truncated) return;
    }
  }

  for (const r of roots) await walk(r);
  return { files: out, truncated };
}
