import type { Draft } from '../types';

const key = (projectId: string, path: string) => `grimoire.knowledge.draft:${projectId}:${path}`;

// Draf per tab (sessionStorage); semua akses dibungkus try/catch agar mode privat/kuota tidak error.
export function loadDraft(projectId: string, path: string): Draft | null {
  try {
    const raw = sessionStorage.getItem(key(projectId, path));
    if (!raw) return null;
    const d = JSON.parse(raw) as Draft;
    return typeof d.content === 'string' && typeof d.baseVersion === 'string' ? d : null;
  } catch {
    return null;
  }
}
export function saveDraft(projectId: string, path: string, d: Draft): void {
  try {
    sessionStorage.setItem(key(projectId, path), JSON.stringify(d));
  } catch {
    /* tanpa draf */
  }
}
export function clearDraft(projectId: string, path: string): void {
  try {
    sessionStorage.removeItem(key(projectId, path));
  } catch {
    /* abaikan */
  }
}
