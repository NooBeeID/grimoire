const INTERACTIVE = 'button,a,input,select,textarea,label';

/** Klik baris membuka detail, kecuali klik pada kontrol di dalam baris atau saat teks baris sedang diseleksi. */
export function shouldOpenFromRowClick(e: Pick<MouseEvent, 'target' | 'currentTarget'>): boolean {
  const target = e.target as Element | null;
  if (target?.closest?.(INTERACTIVE)) return false;
  const sel = typeof window !== 'undefined' ? window.getSelection?.() : null;
  if (sel && !sel.isCollapsed && sel.anchorNode && (e.currentTarget as Node | null)?.contains(sel.anchorNode)) return false;
  return true;
}
