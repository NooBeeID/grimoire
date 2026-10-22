// Parser ringan HANYA untuk UX (auto-isi Branch/Folder, deteksi host, pesan dini). Server otoritatif.
export type UrlProblem = 'required' | 'https_only' | 'userinfo' | 'query' | 'blob_url';

export type ParsedRepoUrl =
  | { ok: true; provider: 'github' | 'gitlab'; host: string; repoPath: string; branch?: string; folder?: string }
  | { ok: false; problem: UrlProblem };

export function parseRepoUrlLite(raw: string): ParsedRepoUrl {
  const text = raw.trim();
  if (text === '') return { ok: false, problem: 'required' };
  if (!/^https:\/\//i.test(text)) return { ok: false, problem: 'https_only' };
  let u: URL;
  try {
    u = new URL(text);
  } catch {
    return { ok: false, problem: 'https_only' };
  }
  if (u.username !== '' || u.password !== '') return { ok: false, problem: 'userinfo' };
  if (u.search !== '' || u.hash !== '' || /[?#]/.test(text)) return { ok: false, problem: 'query' };

  const segs = u.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const provider = u.host === 'github.com' ? 'github' : 'gitlab';
  const stripGit = (s: string) => s.replace(/\.git$/, '');

  if (provider === 'github') {
    if (segs.length < 2) return { ok: true, provider, host: u.host, repoPath: segs.join('/') };
    const repoPath = `${segs[0]}/${stripGit(segs[1]!)}`;
    const mode = segs[2];
    if (mode === 'blob') return { ok: false, problem: 'blob_url' };
    if (mode === 'tree' && segs[3]) return { ok: true, provider, host: u.host, repoPath, branch: segs[3], folder: segs.slice(4).join('/') };
    return { ok: true, provider, host: u.host, repoPath };
  }

  const dash = segs.indexOf('-');
  const repoSegs = dash === -1 ? segs : segs.slice(0, dash);
  if (repoSegs.length > 0) repoSegs[repoSegs.length - 1] = stripGit(repoSegs[repoSegs.length - 1]!);
  const repoPath = repoSegs.join('/');
  if (dash !== -1) {
    const mode = segs[dash + 1];
    if (mode === 'blob') return { ok: false, problem: 'blob_url' };
    if (mode === 'tree' && segs[dash + 2]) {
      return { ok: true, provider, host: u.host, repoPath, branch: segs[dash + 2], folder: segs.slice(dash + 3).join('/') };
    }
  }
  return { ok: true, provider, host: u.host, repoPath };
}
