export const meta = {
  name: 'test',
  description: 'Verify-and-fix loop: jalankan build/vet/test/lint di worktree, perbaiki kegagalan per stack, ulangi sampai maxAttempts',
  whenToUse: 'Child workflow dari new-feature setelah implement-feature',
  phases: [{ title: 'Verify', detail: 'build + test + coverage, fix per stack, ulangi' }],
}

// args: { planPath, worktreePath, scope, maxAttempts=3, slug, minCoverage? }
// return: { status: 'DONE'|'EVAL_EXHAUSTED'|'NEEDS_HUMAN_INPUT', attempts, coverage?, failures?, reason? }

const a = args || {}
if (!a.worktreePath || !a.planPath) return { status: 'NEEDS_HUMAN_INPUT', reason: 'planPath dan worktreePath wajib diisi' }

const wt = a.worktreePath
const scope = a.scope || 'fullstack'
const maxAttempts = Number.isInteger(a.maxAttempts) && a.maxAttempts > 0 ? a.maxAttempts : 3
const minCoverage = typeof a.minCoverage === 'number' ? a.minCoverage : null
const slug = a.slug || 'feature'
const hasBackend = scope === 'backend' || scope === 'fullstack'
const hasFrontend = scope === 'frontend' || scope === 'fullstack'

const WT = `WORKTREE: kerja hanya di ${wt} (cd ke sana, path absolut). Jangan menyentuh checkout utama.`

const VERIFY_SCHEMA = {
  type: 'object',
  properties: {
    passed: { type: 'boolean' },
    coverage: { type: 'number', description: 'persentase coverage backend (go test -cover, rata-rata paket baru); 0 bila tidak diukur' },
    failures: {
      type: 'array',
      items: {
        type: 'object',
        properties: { stack: { type: 'string', enum: ['backend', 'frontend'] }, command: { type: 'string' }, summary: { type: 'string' }, output: { type: 'string' } },
        required: ['stack', 'command', 'summary'],
      },
    },
    infraProblem: { type: 'string', description: 'isi bila environment rusak (mis. Postgres tidak bisa dijalankan), bukan kegagalan kode' },
  },
  required: ['passed', 'failures'],
}

const verifyPrompt = () => `${WT}
Jalankan verifikasi lengkap untuk scope ${scope} dan laporkan hasilnya. Jangan memperbaiki apa pun.
${hasBackend ? `Backend (dari ${wt}/backend): go build ./... ; go vet ./... ; go test ./... -cover. Test integrasi butuh Postgres: jika docker-compose.dev.yml ada dan Postgres belum jalan, jalankan "docker compose -f docker-compose.dev.yml up -d" dulu. Jika Postgres tidak bisa dijalankan, isi infraProblem.` : ''}
${hasFrontend ? `Frontend (dari ${wt}/frontend): jalankan skrip yang ada di package.json untuk check/typecheck, lint, test, dan build (npm run ...). Jika node_modules belum ada, npm install dulu.` : ''}
Cocokkan hasil dengan skenario di plan ${a.planPath} (Test Scenario dan E2E Scenarios): jika ada skenario yang jelas belum punya test, catat sebagai failure stack terkait dengan command "coverage-of-plan".
${minCoverage != null ? `Coverage minimum: ${minCoverage}%. Jika coverage di bawah itu, passed=false dengan failure command "coverage".` : 'Tidak ada coverage minimum.'}
passed=true hanya bila semua perintah sukses dan failures kosong. Potong output panjang menjadi bagian yang relevan (maks sekitar 60 baris per failure).`

const fixPrompt = (stack, list) => `${WT}
Perbaiki kegagalan berikut di stack ${stack}. Plan acuan: ${a.planPath}. Perbaiki akar masalah di kode atau test yang salah; jangan menghapus atau melonggarkan test agar lolos, dan jangan mengubah plan. Jalankan ulang perintah yang gagal untuk memastikan lolos.
${list.map((f, i) => `${i + 1}. [${f.command}] ${f.summary}\n${f.output || ''}`).join('\n\n')}
Jika kegagalan disebabkan requirement yang ambigu, bukan bug, set needsHumanInput=true dan isi reason. Setelah selesai, commit: git add -A && git commit -m "fix(${slug}): verify loop" dengan baris terakhir pesan "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>". Jangan push.`

const FIX_SCHEMA = {
  type: 'object',
  properties: { summary: { type: 'string' }, needsHumanInput: { type: 'boolean' }, reason: { type: 'string' } },
  required: ['summary'],
}

phase('Verify')
let last = null
for (let attempt = 1; attempt <= maxAttempts; attempt++) {
  log(`Verify attempt ${attempt}/${maxAttempts}`)
  const res = await agent(verifyPrompt(), { label: `verify-${attempt}`, model: 'sonnet', schema: VERIFY_SCHEMA })
  if (res == null) return { status: 'NEEDS_HUMAN_INPUT', reason: `agent verify-${attempt} gagal (kegagalan infrastruktur)`, attempts: attempt }
  if (res.infraProblem) return { status: 'NEEDS_HUMAN_INPUT', reason: `environment: ${res.infraProblem}`, attempts: attempt }
  last = res
  if (res.passed && res.failures.length === 0) return { status: 'DONE', attempts: attempt, coverage: res.coverage }
  if (attempt === maxAttempts) break

  const stacks = [...new Set(res.failures.map((f) => f.stack))]
  const fixes = await parallel(stacks.map((st) => () => agent(
    fixPrompt(st, res.failures.filter((f) => f.stack === st)),
    { label: `fix-${st}-${attempt}`, agentType: st === 'backend' ? 'backend-developer' : 'frontend-developer', schema: FIX_SCHEMA })))
  if (fixes.some((f) => f == null)) return { status: 'NEEDS_HUMAN_INPUT', reason: 'agent fix gagal (kegagalan infrastruktur)', attempts: attempt }
  const human = fixes.find((f) => f.needsHumanInput)
  if (human) return { status: 'NEEDS_HUMAN_INPUT', reason: human.reason || 'fix butuh input manusia', attempts: attempt, failures: res.failures }
}

return { status: 'EVAL_EXHAUSTED', attempts: maxAttempts, coverage: last ? last.coverage : undefined, failures: last ? last.failures : [] }
