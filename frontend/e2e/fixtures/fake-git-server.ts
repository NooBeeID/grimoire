import { createHash } from 'node:crypto';
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import { fileURLToPath } from 'node:url';

// Server GitHub palsu untuk e2e git-sync: GET /user, git trees, dan blobs, plus endpoint kontrol
// untuk mengganti isi repo antar langkah. Backend e2e dijalankan dengan
// GIT_SYNC_INSECURE_TEST_HOSTS=127.0.0.1:<port> dan GIT_SYNC_GITHUB_API_BASE=http://127.0.0.1:<port>.
// Hanya sintaks yang bisa dijalankan Node langsung (type stripping): `node e2e/fixtures/fake-git-server.ts`.

export const FAKE_GIT_PORT = Number(process.env.E2E_FAKE_GIT_PORT ?? 8089);
export const FAKE_GIT_URL = `http://127.0.0.1:${FAKE_GIT_PORT}`;
export const VALID_TOKEN = 'ghp_e2eValidToken7Qx2';

interface Repo {
  branch: string;
  files: Map<string, string>;
  rateLimit: boolean | RateLimitOpt;
  status429Once: boolean;
  blobDelayMs: number;
  /** Jumlah request API ke repo ini (tree + blob) sejak konfigurasi terakhir. */
  requests: number;
  limitedUntil: number;
  sent429: boolean;
}

export interface RateLimitOpt {
  /** Setelah sekian request sukses, request berikutnya kena limit. */
  afterRequests: number;
  /** Lama jendela limit (detik); header x-ratelimit-reset dan retry-after mengikutinya. */
  resetSeconds: number;
}

export interface RepoState {
  branch?: string;
  /** path -> isi; menggantikan seluruh isi repo. */
  files?: Record<string, string>;
  /** true: tree mengembalikan 403 dengan x-ratelimit-remaining: 0. */
  rateLimit?: boolean | RateLimitOpt;
  /** true: request pertama ke repo dibalas 429 (retry-after 1) sekali saja. */
  status429Once?: boolean;
  /** Jeda (ms) sebelum tiap blob dikirim, untuk sync yang berjalan lama. */
  blobDelayMs?: number;
}

const blobSha = (content: string) => createHash('sha1').update(`blob ${Buffer.byteLength(content)}\0`).update(content).digest('hex');

function send(res: ServerResponse, status: number, body: unknown, headers: Record<string, string> = {}) {
  const raw = typeof body === 'string' ? body : JSON.stringify(body);
  res.writeHead(status, { 'content-type': typeof body === 'string' ? 'text/plain; charset=utf-8' : 'application/json', ...headers });
  res.end(raw);
}

async function readJson(req: IncomingMessage): Promise<RepoState> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  const text = Buffer.concat(chunks).toString('utf8');
  return text ? (JSON.parse(text) as RepoState) : {};
}

interface Limited {
  status: number;
  message: string;
  headers: Record<string, string>;
}

/** Hitung request dan putuskan apakah repo sedang kena rate limit (429 sekali, jendela limit, atau limit tetap). */
function checkLimits(repo: Repo): Limited | null {
  repo.requests += 1;
  if (repo.status429Once && !repo.sent429) {
    repo.sent429 = true;
    return { status: 429, message: 'Too Many Requests', headers: { 'retry-after': '1' } };
  }
  const rl = repo.rateLimit;
  if (rl === true) return { status: 403, message: 'API rate limit exceeded', headers: { 'x-ratelimit-remaining': '0', 'retry-after': '30' } };
  if (rl && typeof rl === 'object') {
    const now = Date.now();
    if (repo.limitedUntil === 0 && repo.requests > rl.afterRequests) repo.limitedUntil = now + rl.resetSeconds * 1000;
    if (repo.limitedUntil > now) {
      const secs = Math.max(1, Math.ceil((repo.limitedUntil - now) / 1000));
      return {
        status: 403,
        message: 'API rate limit exceeded',
        headers: { 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': String(Math.ceil(repo.limitedUntil / 1000)), 'retry-after': String(secs) },
      };
    }
  }
  return null;
}

export function createFakeGitServer(): Server {
  // Kunci repo: "owner/repo". Tiap tes memakai nama repo unik agar paralel aman.
  const repos = new Map<string, Repo>();
  const blobs = new Map<string, string>();

  const authorized = (req: IncomingMessage) => req.headers.authorization === `Bearer ${VALID_TOKEN}`;

  return createServer((req, res) => {
    void (async () => {
      const url = new URL(req.url ?? '/', FAKE_GIT_URL);
      const parts = url.pathname.split('/').filter(Boolean).map(decodeURIComponent);

      if (url.pathname === '/healthz') return send(res, 200, { ok: true });

      // Kontrol: PUT /__control/repos/{owner}/{repo}
      if (parts[0] === '__control' && parts[1] === 'repos' && parts.length === 4 && req.method === 'PUT') {
        const body = await readJson(req);
        const key = `${parts[2]}/${parts[3]}`;
        const prev = repos.get(key);
        const files = body.files ? new Map(Object.entries(body.files)) : (prev?.files ?? new Map<string, string>());
        for (const content of files.values()) blobs.set(blobSha(content), content);
        repos.set(key, {
          branch: body.branch ?? prev?.branch ?? 'main',
          files,
          rateLimit: body.rateLimit ?? prev?.rateLimit ?? false,
          status429Once: body.status429Once ?? prev?.status429Once ?? false,
          blobDelayMs: body.blobDelayMs ?? prev?.blobDelayMs ?? 0,
          requests: 0,
          limitedUntil: 0,
          sent429: false,
        });
        return send(res, 200, { ok: true });
      }

      if (req.method !== 'GET') return send(res, 405, { message: 'method not allowed' });

      if (!authorized(req)) return send(res, 401, { message: 'Bad credentials' });

      if (url.pathname === '/user') return send(res, 200, { login: 'grimoire-e2e', id: 1 });

      // GET /repos/{o}/{r}/git/trees/{branch}?recursive=1
      if (parts[0] === 'repos' && parts[3] === 'git' && parts[4] === 'trees' && parts.length === 6) {
        const repo = repos.get(`${parts[1]}/${parts[2]}`);
        if (!repo || repo.branch !== parts[5]) return send(res, 404, { message: 'Not Found' });
        const limited = checkLimits(repo);
        if (limited) return send(res, limited.status, { message: limited.message }, limited.headers);
        const tree = [...repo.files.entries()]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([path, content]) => ({ path, mode: '100644', type: 'blob', sha: blobSha(content), size: Buffer.byteLength(content) }));
        return send(res, 200, { sha: 'tree', truncated: false, tree });
      }

      // GET /repos/{o}/{r}/git/blobs/{sha} (Accept: application/vnd.github.raw)
      if (parts[0] === 'repos' && parts[3] === 'git' && parts[4] === 'blobs' && parts.length === 6) {
        const repo = repos.get(`${parts[1]}/${parts[2]}`);
        if (!repo) return send(res, 404, { message: 'Not Found' });
        const limited = checkLimits(repo);
        if (limited) return send(res, limited.status, { message: limited.message }, limited.headers);
        if (repo.blobDelayMs > 0) await new Promise((r) => setTimeout(r, repo.blobDelayMs));
        const content = blobs.get(parts[5] ?? '');
        if (content === undefined) return send(res, 404, { message: 'Not Found' });
        res.writeHead(200, { 'content-type': 'application/vnd.github.raw' });
        return res.end(content);
      }

      return send(res, 404, { message: 'Not Found' });
    })().catch(() => send(res, 500, { message: 'fixture error' }));
  });
}

/** Klien kontrol untuk tes: ganti isi repo (menggantikan seluruh file) atau nyalakan mode rate limit. */
export async function setRepo(repo: string, state: RepoState): Promise<void> {
  const res = await fetch(`${FAKE_GIT_URL}/__control/repos/${repo}`, { method: 'PUT', body: JSON.stringify(state) });
  if (!res.ok) throw new Error(`fake-git-server: ${res.status} ${await res.text()}`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  createFakeGitServer().listen(FAKE_GIT_PORT, '127.0.0.1', () => {
    console.log(`fake-git-server mendengarkan di ${FAKE_GIT_URL}`);
  });
}
