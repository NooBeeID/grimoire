import { sanitizeSvg } from './sanitize';

export const MERMAID_MAX_CHARS = 50_000;

export type MermaidResult = { ok: true; svg: string } | { ok: false };

type MermaidApi = typeof import('mermaid').default;
let loader: Promise<MermaidApi> | null = null;

// Impor lazy sekali (chunk terpisah); initialize juga sekali.
function load(): Promise<MermaidApi> {
  loader ??= import('mermaid').then((m) => {
    const api = m.default;
    api.initialize({ startOnLoad: false, securityLevel: 'strict', flowchart: { htmlLabels: false }, theme: 'neutral' });
    return api;
  });
  loader.catch(() => {
    loader = null; // izinkan coba lagi setelah gagal muat
  });
  return loader;
}

export async function renderMermaid(code: string, id: string): Promise<MermaidResult> {
  if (code.length > MERMAID_MAX_CHARS) return { ok: false };
  try {
    const api = await load();
    const { svg } = await api.render(id, code);
    return { ok: true, svg: sanitizeSvg(svg) };
  } catch {
    return { ok: false };
  }
}
