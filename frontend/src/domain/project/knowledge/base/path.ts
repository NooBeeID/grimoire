import { hasControlChars, runeLength } from '../../../../shared/utils/text';
import type { Reason } from './types';

// Salinan perilaku backend base/path.go; backend tetap otoritatif.
export const MAX_PATH_RUNES = 512;
export const MAX_SEGMENT_RUNES = 255;
export const SUPPORTED_EXTS = ['.md', '.markdown', '.txt'] as const;

export function validatePath(raw: string): Reason | null {
  if (raw === '') return 'path_empty';
  if (hasControlChars(raw)) return 'path_control_char';
  if (raw.startsWith('/')) return 'path_absolute';
  if (runeLength(raw) > MAX_PATH_RUNES) return 'path_too_long';
  const segs = raw.split('/');
  for (const s of segs) {
    if (s === '.' || s === '..') return 'path_traversal';
  }
  for (const s of segs) {
    if (s === '') return 'path_empty_segment';
    if (runeLength(s) > MAX_SEGMENT_RUNES) return 'segment_too_long';
  }
  return null;
}

export function normalizePrefix(raw: string): string {
  return raw.endsWith('/') ? raw.slice(0, -1) : raw;
}
export function validatePrefix(raw: string): Reason | null {
  const p = normalizePrefix(raw);
  return p === '' ? null : validatePath(p);
}

export function isHidden(p: string): boolean {
  return p.split('/').some((s) => s.startsWith('.'));
}
export function baseName(p: string): string {
  return p.slice(p.lastIndexOf('/') + 1);
}
export function dirName(p: string): string {
  const i = p.lastIndexOf('/');
  return i < 0 ? '' : p.slice(0, i);
}
export function ext(p: string): string {
  const b = baseName(p);
  const i = b.lastIndexOf('.');
  return i <= 0 ? '' : b.slice(i).toLowerCase();
}
export function isSupported(p: string): boolean {
  return (SUPPORTED_EXTS as readonly string[]).includes(ext(p));
}
export function isMarkdown(p: string): boolean {
  const e = ext(p);
  return e === '.md' || e === '.markdown';
}
export function ancestors(p: string): string[] {
  const segs = p.split('/').slice(0, -1);
  return segs.map((_, i) => segs.slice(0, i + 1).join('/'));
}
export function joinPath(dir: string, rel: string): string {
  return dir === '' ? rel : `${normalizePrefix(dir)}/${rel}`;
}
export function validateNewFilePath(raw: string): Reason | null {
  const r = validatePath(raw);
  if (r) return r;
  if (isHidden(raw)) return 'hidden';
  if (!isSupported(raw)) return 'unsupported_type';
  return null;
}
