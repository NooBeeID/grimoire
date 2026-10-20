export const meta = {
  name: 'implement-feature',
  description: 'Buat worktree, implementasi plan: foundation -> usecase paralel (dibatasi) -> wiring -> completeness gate; FE slicing paralel dengan backend',
  whenToUse: 'Child workflow dari new-feature. Menerima planPath lokal dan mengembalikan IMPLEMENTED atau NEEDS_HUMAN_INPUT',
  phases: [
    { title: 'Setup', detail: 'git worktree add, infer scope' },
    { title: 'Backend', detail: 'decompose -> foundation -> usecase paralel -> wiring' },
    { title: 'Frontend', detail: 'slicing dengan mock (fullstack: paralel dengan backend) lalu wiring' },
    { title: 'Completeness', detail: 'cek gap, fix per stack, maks 2 putaran' },
  ],
}

// args: { planPath, scope?, slug?, base?, notes?, maxAgentParalel? }

class Halt extends Error {}
const halt = (reason) => { throw new Halt(reason) }
const must = (v, what) => { if (v == null) halt(`Agent ${what} gagal (hasil null, kegagalan infrastruktur)`); return v }

const a = args || {}
const SCOPES = ['backend', 'frontend', 'fullstack']
const DEFAULT_MAX_PARALLEL_USECASES = 3
const MAX_COMPLETENESS_ATTEMPTS = 2

function slugFromPlanPath(p) {
  const parts = String(p).split('/').filter(Boolean)
  const file = parts.pop() || ''
  const stripDate = (s) => s.replace(/^\d{4}-\d{2}-\d{2}-/, '')
  let name = stripDate(file.replace(/\.md$/i, ''))
  if (['overview', 'plan', 'index'].includes(name.toLowerCase())) name = stripDate(parts.pop() || name)
  return name.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/-+/g, '-').replace(/^-+|-+$/g, '')
}

async function runWithLimit(items, worker, limit) {
  const results = new Array(items.length); let next = 0
  async function runNext() { while (next < items.length) { const i = next++; results[i] = await worker(items[i], i) } }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, runNext))
  return results
}

if (!a.planPath) return { status: 'NEEDS_HUMAN_INPUT', reason: 'planPath wajib diisi' }
if (a.scope && !SCOPES.includes(a.scope)) return { status: 'NEEDS_HUMAN_INPUT', reason: `scope tidak valid: ${a.scope}`, planPath: a.planPath }

const slug = a.slug || slugFromPlanPath(a.planPath)
const branch = `feat/${slug}`
const progressRel = `.superpowers/sdd/${slug}/progress.md`

const CONTEXT_LOADING = `Muat konteks dengan urutan: CLAUDE.md (jika ada) -> .claude/rules/ (jika ada) -> docs/tickets/README.md (konvensi arsitektur) -> knowledge doc module terkait. Lalu baca plan.`
const NO_STOPPING_NOTE = `Selesaikan semuanya dalam satu turn. needsHumanInput=true hanya untuk ambiguitas requirement yang nyata (isi reason), bukan pertanyaan proses.`
const BUILD_NOTE = `Perintah verifikasi backend: "go build ./..." dan "go vet ./..." dari folder backend/. Frontend: skrip check/lint/test/build di frontend/package.json. Perbaiki sendiri error yang muncul dari pekerjaanmu.`

const RESULT = {
  type: 'object',
  properties: { summary: { type: 'string' }, needsHumanInput: { type: 'boolean' }, reason: { type: 'string' } },
  required: ['summary'],
}
const OK = { type: 'object', properties: { ok: { type: 'boolean' } }, required: ['ok'] }

const checkHuman = (r, what) => { if (r && r.needsHumanInput) halt(`${what}: ${r.reason || 'butuh input manusia'}`); return r }

let wt = '' // absolute worktree path, diisi setelah Setup
const warnings = []
const WT = () => `WORKTREE: semua pekerjaan dilakukan di ${wt}. Gunakan path absolut di bawah folder itu untuk Read/Write/Edit dan jalankan perintah dengan cd ${wt}. Jangan menyentuh checkout utama.`

async function progress(text) {
  await agent(
    `Edit ${wt}/${progressRel} (buat dengan header dan tabel kolom "Status | Tahap | Catatan" jika belum ada). Sisipkan satu baris baru di PALING ATAS tabel (di bawah header): "${text}". Jangan mengubah urutan atau menghapus baris lama.`,
    { label: 'progress', model: 'haiku', effort: 'low', schema: OK })
}

async function runBody() {
  // ---------------------------------------------------------------- Setup
  phase('Setup')
  const setup = must(await agent(
    `Setup worktree untuk feature "${slug}".
1. Pastikan file plan ada: ${a.planPath}. Jika tidak ada, ok=false.
2. Pastikan ./worktree/ ter-exclude dari git (tambahkan "worktree/" ke .git/info/exclude jika belum).
3. Jika worktree ./worktree/feat-${slug} belum ada: git worktree add ./worktree/feat-${slug} -b ${branch}${a.base ? ` ${a.base}` : ''}. Jika sudah ada (resume), pakai apa adanya.
4. Worktree di-fork dari HEAD, jadi plan harus ada di worktree. Jika ${a.planPath} (dan folder induknya bila plan terdekomposisi, yaitu overview.md) belum ada di worktree, salin dari checkout utama ke path relatif yang sama di worktree.
5. Infer scope dari plan jika belum ditentukan${a.scope ? ` (sudah ditentukan: ${a.scope}, pakai itu)` : ''}: lihat tabel Metadata; ada bagian "## Frontend Plan" atau frontend-plan.md bersama plan backend = fullstack; hanya backend = backend; hanya frontend = frontend.
6. Kembalikan path absolut worktree.`,
    {
      label: 'worktree-setup', model: 'haiku', effort: 'low',
      schema: {
        type: 'object',
        properties: { ok: { type: 'boolean' }, reason: { type: 'string' }, worktreePath: { type: 'string' }, scope: { type: 'string', enum: SCOPES } },
        required: ['ok'],
      },
    }), 'worktree-setup')
  if (!setup.ok) halt(setup.reason || 'setup worktree gagal')
  wt = setup.worktreePath
  const scope = a.scope || setup.scope
  if (!SCOPES.includes(scope)) halt('scope tidak bisa di-infer dari plan')
  const hasBackend = scope === 'backend' || scope === 'fullstack'
  const hasFrontend = scope === 'frontend' || scope === 'fullstack'
  const NOTES = a.notes ? `\nCatatan tambahan dari pemanggil:\n${a.notes}` : ''
  const PLAN = `Plan: ${a.planPath} (jika plan terdekomposisi, folder induknya berisi overview.md, usecase-*.md, frontend-plan.md).`
  await progress(`| IN PROGRESS | setup | worktree ${wt}, branch ${branch}, scope ${scope} |`)

  // ------------------------------------------------------- Backend pipeline
  async function backendPipeline() {
    const decomp = must(await agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nBaca plan lalu pecah implementasi backend. Tentukan resourceRootPath (folder module, mis. backend/internal/domain/<domain>/internal/<module>), foundationSummary (isi folder base/ dan kontrak bersama yang harus selesai dulu: entity, konstanta bus lib/constants/bus.go, tipe di lib/contracts), dan daftar usecase (slug, deskripsi, planPath file usecase-<slug>.md; jika plan tunggal, 1 usecase dengan planPath = plan itu). Untuk tiap usecase isi allowedPaths: path repo-relatif (folder berakhiran '/', atau file) yang BOLEH ditulis usecase itu, diturunkan dari bagian Changes di plan usecase-nya (bisa di luar resource root, mis. backend/cmd/, api/, frontend/, makefile). allowedPaths antar usecase TIDAK boleh tumpang tindih dan tidak boleh mencakup folder base/ atau base.go. Set runLate=true untuk usecase yang bergantung pada hasil usecase lain atau mengedit file gabungan (mis. CLI/wiring yang meng-import usecase lain). Jangan menulis kode.`,
      {
        label: 'decompose-backend', model: 'sonnet', agentType: 'backend-architect',
        schema: {
          type: 'object',
          properties: {
            resourceRootPath: { type: 'string' },
            foundationSummary: { type: 'string' },
            usecases: { type: 'array', items: { type: 'object', properties: { slug: { type: 'string' }, description: { type: 'string' }, planPath: { type: 'string' }, allowedPaths: { type: 'array', items: { type: 'string' } }, runLate: { type: 'boolean' } }, required: ['slug', 'description', 'planPath', 'allowedPaths'] } },
          },
          required: ['resourceRootPath', 'foundationSummary', 'usecases'],
        },
      }), 'decompose-backend')
    if (!decomp.usecases.length) halt('decompose-backend tidak menghasilkan usecase')
    const root = decomp.resourceRootPath
    const ucs = decomp.usecases
    for (const u of ucs) if (!u.allowedPaths.length) u.allowedPaths = [`${root}/${u.slug}/`]
    for (const u of ucs) for (const v of ucs) {
      if (u === v) continue
      for (const p of u.allowedPaths) for (const q of v.allowedPaths) {
        const dir = (x) => (x.endsWith('/') ? x : `${x}/`)
        if (p === q || dir(q).startsWith(dir(p)) || (p.endsWith('/') && q.startsWith(p))) halt(`allowedPaths tumpang tindih: ${u.slug} (${p}) dan ${v.slug} (${q})`)
      }
    }

    // Foundation: HANYA base/ + skeleton base.go dengan slice usecase kosong
    const foundation = checkHuman(must(await agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nImplementasi FOUNDATION saja untuk module di ${root}:\n- ${root}/base/ (entity + business rule, tanpa I/O)\n- skeleton ${root}/base.go: wiring module dengan slice usecase KOSONG (usecase didaftarkan nanti oleh tahap wiring)\n- konstanta endpoint bus di backend/lib/constants/bus.go dan tipe kontrak di backend/lib/contracts/** yang dibutuhkan semua usecase\n- migrasi SQL bila plan menyebutnya\nRingkasan foundation: ${decomp.foundationSummary}\nDaftar usecase (JANGAN diimplementasi sekarang): ${ucs.map((u) => u.slug).join(', ')}.\nHasil harus compile (go build ./...). Setelah compile, commit di worktree: git add -A && git commit -m "wip(${slug}): foundation" dan kembalikan SHA-nya.\n${BUILD_NOTE}\n${NO_STOPPING_NOTE}`,
      {
        label: 'implement-backend-foundation', agentType: 'backend-developer',
        schema: { type: 'object', properties: { summary: { type: 'string' }, foundationSha: { type: 'string' }, needsHumanInput: { type: 'boolean' }, reason: { type: 'string' } }, required: ['summary', 'foundationSha'] },
      }), 'implement-backend-foundation'), 'foundation')
    await progress(`| DONE | backend foundation | sha ${foundation.foundationSha} |`)

    // Fan-out usecase, dibatasi runWithLimit
    let limit = DEFAULT_MAX_PARALLEL_USECASES
    if (a.maxAgentParalel === 'full') limit = ucs.length
    else if (Number.isInteger(a.maxAgentParalel) && a.maxAgentParalel > 0) limit = a.maxAgentParalel
    log(`Fan-out ${ucs.length} usecase, maks ${limit} agent bersamaan`)

    const runUc = (u) => agent(
      `${WT()}\n${CONTEXT_LOADING}\n${NOTES}\n\nImplementasi SATU usecase: "${u.slug}" - ${u.description}\nPlan usecase: ${u.planPath}\nFoundation sudah selesai dan compile (${root}/base, konstanta bus, kontrak); import saja.${u.runLate ? ' Usecase lain sudah selesai dan boleh di-import; kamu berjalan terakhir.' : ''}\n\nATURAN ISOLASI (wajib, ada agent lain yang bekerja paralel di worktree yang sama):\n1. Hanya menulis di path berikut (kode + test untuk skenario positif dan negatif dari plan): ${u.allowedPaths.join(', ')}. Jika plan butuh menulis di luar daftar ini, JANGAN menulis; kerjakan sisanya dan sebutkan di summary. Jika pekerjaan usecase sudah ada di worktree (dari run sebelumnya), cukup verifikasi build/test paketnya. Nama folder bisa berbeda dari slug; ikuti plan. needsHumanInput=true HANYA untuk requirement yang benar-benar ambigu, bukan karena path, nama folder, atau perintah verifikasi.\n2. Dilarang menyentuh ${root}/base/ (hanya di-import) dan ${root}/base.go.\n3. Dilarang menyentuh folder usecase lain.\n4. Jangan mendaftarkan route atau handler bus di base.go; itu dikerjakan satu agent di tahap wiring. Sediakan konstruktor/fungsi registrasi di init.go folder usecase-mu.\n5. Boleh menambah file BARU (bukan mengubah yang ada) di backend/lib/contracts/** milik module ini dan di backend/migrations/ bila plan mewajibkannya. Jangan mengubah backend/lib/constants/bus.go.\n6. Jangan git commit (agent lain berjalan bersamaan).\n${BUILD_NOTE}\nJalankan "go build ./..." dan go test untuk paket-paket yang kamu tulis.\n${NO_STOPPING_NOTE}`,
      { label: `implement-backend-${u.slug}`, agentType: 'backend-developer', schema: RESULT })
    const early = ucs.filter((u) => !u.runLate)
    const late = ucs.filter((u) => u.runLate)
    const ucResults = await runWithLimit(early, runUc, limit)
    const lateResults = late.length ? await runWithLimit(late, runUc, limit) : []
    const ucFailures = []
    ;[...early, ...late].forEach((u, i) => {
      const r = [...ucResults, ...lateResults][i]
      if (r == null) halt(`Agent implement-backend-${u.slug} gagal (hasil null, kegagalan infrastruktur)`)
      if (r.needsHumanInput) ucFailures.push(`usecase ${u.slug}: ${r.reason || 'butuh input manusia'}`)
    })
    // needsHumanInput dari usecase = peringatan; completeness gate yang memutuskan lanjut/berhenti
    ucFailures.forEach((f) => { warnings.push(f); log(`Peringatan: ${f}`) })

    // Guard isolasi: tidak ada diff di base/ atau base.go selama fan-out
    const iso = must(await agent(
      `${WT()}\nPeriksa isolasi fan-out. Jalankan "git -C ${wt} status --porcelain" dan "git -C ${wt} diff --name-only ${foundation.foundationSha}". Pelanggaran = path yang berubah/baru DAN bukan salah satu dari: (a) salah satu allowedPaths usecase: ${ucs.map((u) => `${u.slug}=[${u.allowedPaths.join(', ')}]`).join('; ')}, (b) file BARU di backend/lib/contracts/**, (c) file BARU di backend/migrations/, (d) .superpowers/**. Perubahan apa pun di ${root}/base/, ${root}/base.go, atau backend/lib/constants/bus.go juga pelanggaran. Untuk rename/pemindahan (status R) atau penghapusan: path asal dianggap sah bila path tujuan rename ada di salah satu allowedPaths, karena itu usecase merapikan hasil kerjanya sendiri. Hanya penghapusan murni file milik usecase lain, atau perubahan di base/, base.go, bus.go, yang pelanggaran. Jangan memperbaiki apa pun, hanya laporkan.`,
      { label: 'isolation-check', model: 'haiku', effort: 'low', schema: { type: 'object', properties: { violations: { type: 'array', items: { type: 'string' } } }, required: ['violations'] } }), 'isolation-check')
    if (iso.violations.length) halt(`isolasi usecase dilanggar: ${iso.violations.join(', ')}`)
    await progress(`| DONE | backend usecases (${ucs.length}) | limit ${limit} |`)

    // Wiring: satu agent
    checkHuman(must(await agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nTAHAP WIRING backend untuk module di ${root}. Semua usecase (${ucs.map((u) => u.slug).join(', ')}) sudah diimplementasi di foldernya masing-masing. Jika ${root}/base.go tidak ada (mis. root infra, bukan module domain), tahap ini hanya memastikan semua usecase tersambung, go build/test hijau, dan sisa item plan yang belum tercakup selesai. Jika ada, kamu satu-satunya yang mengedit ${root}/base.go: daftarkan semua usecase (route REST + handler bus), pastikan registrasi ganda tidak terjadi, hubungkan module ke wiring domain (backend/internal/domain/<domain>/base.go) bila perlu, dan perbarui knowledge doc module jika ada. Jalankan "go build ./..." dan "go test ./..." dari backend/ lalu perbaiki yang gagal.\n${NO_STOPPING_NOTE}`,
      { label: 'implement-backend-wiring', agentType: 'backend-developer', schema: RESULT }), 'implement-backend-wiring'), 'wiring backend')
    await progress(`| DONE | backend wiring | go build ./... |`)
    return { root, usecases: ucs }
  }

  // ------------------------------------------------------------ FE helpers
  const feSlicing = async () => checkHuman(must(await agent(
    `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nFE SLICING${scope === 'fullstack' ? ' (backend sedang dikerjakan paralel oleh agent lain)' : ''}: buat route, skeleton komponen, dan state scaffolding di frontend/src/domain/** dengan DATA MOCK. Pakai api/modules/** sebagai kontrak dan buat services/ yang memvalidasi respons dengan Zod terhadap mock. Dilarang menyentuh backend/.\n${BUILD_NOTE}\n${NO_STOPPING_NOTE}`,
    { label: 'implement-frontend-slicing', agentType: 'frontend-developer', schema: RESULT }), 'implement-frontend-slicing'), 'FE slicing')

  // -------------------------------------------------------------- Pipeline
  let backendInfo = null
  if (scope === 'backend') {
    phase('Backend')
    backendInfo = await backendPipeline()
  } else if (scope === 'frontend') {
    phase('Frontend')
    checkHuman(must(await agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nImplementasi frontend lengkap sesuai plan, memakai API nyata dari kontrak api/modules/**. Tulis test Vitest untuk services dan komponen utama.\n${BUILD_NOTE}\n${NO_STOPPING_NOTE}`,
      { label: 'implement-frontend', agentType: 'frontend-developer', schema: RESULT }), 'implement-frontend'), 'implement-frontend')
  } else {
    phase('Backend')
    // Dua jalur bersamaan: barrier genuine, FE wiring butuh keduanya.
    const guard = (fn) => async () => { try { return await fn() } catch (e) { if (e instanceof Halt) return { __halt: e.message }; throw e } }
    const [be, fe] = await parallel([guard(backendPipeline), guard(feSlicing)])
    if (be == null || fe == null) halt('salah satu jalur fullstack gagal (null)')
    if (be.__halt) halt(`backend: ${be.__halt}`)
    if (fe.__halt) halt(`FE slicing: ${fe.__halt}`)
    backendInfo = be
    phase('Frontend')
    await progress(`| DONE | FE slicing + backend pipeline | lanjut FE wiring |`)
    checkHuman(must(await agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}${NOTES}\n\nFE WIRING: backend dan FE slicing sudah selesai. Ganti data mock dengan panggilan API nyata (sesuai api/modules/** dan implementasi backend yang sekarang ada), sambungkan state dan error handling, dan tulis test Vitest. Dilarang menyentuh backend/ kecuali menemukan bug kontrak: laporkan lewat needsHumanInput, jangan mengubah backend.\n${BUILD_NOTE}\n${NO_STOPPING_NOTE}`,
      { label: 'implement-frontend-wiring', agentType: 'frontend-developer', schema: RESULT }), 'implement-frontend-wiring'), 'FE wiring')
  }
  await progress(`| DONE | implementasi | masuk completeness gate |`)

  // ----------------------------------------------------------- Completeness
  phase('Completeness')
  const check = async () => must(await agent(
    `${WT()}\n${CONTEXT_LOADING}\n${PLAN}\n\nBandingkan plan dengan isi worktree. Setiap item di Plan, Changes, Migration, Test Scenario, dan Request/Response harus ada implementasinya (scope: ${scope}). Jalankan build backend/frontend yang relevan. Laporkan gap konkret (stack: backend|frontend, item). Jangan memperbaiki apa pun.`,
    {
      label: 'completeness-check', model: 'sonnet',
      schema: {
        type: 'object',
        properties: {
          complete: { type: 'boolean' },
          gaps: { type: 'array', items: { type: 'object', properties: { stack: { type: 'string', enum: ['backend', 'frontend'] }, item: { type: 'string' } }, required: ['stack', 'item'] } },
        },
        required: ['complete', 'gaps'],
      },
    }), 'completeness-check')

  let result = await check()
  for (let round = 1; !result.complete && round <= MAX_COMPLETENESS_ATTEMPTS; round++) {
    const stacks = [...new Set(result.gaps.map((g) => g.stack))]
    log(`Gap ditemukan (putaran ${round}/${MAX_COMPLETENESS_ATTEMPTS}) di: ${stacks.join(', ')}`)
    // Tanpa re-decompose, tanpa re-fan-out: 1 agent fix per stack, hanya item yang kurang
    await parallel(stacks.map((st) => () => agent(
      `${WT()}\n${CONTEXT_LOADING}\n${PLAN}\n\nPerbaiki HANYA item yang kurang di stack ${st}:\n${result.gaps.filter((g) => g.stack === st).map((g) => `- ${g.item}`).join('\n')}\nJangan mengerjakan ulang yang sudah ada dan jangan menyentuh stack lain.\n${BUILD_NOTE}\n${NO_STOPPING_NOTE}`,
      { label: `fix-${st}-${round}`, agentType: st === 'backend' ? 'backend-developer' : 'frontend-developer', schema: RESULT })
      .then((r) => checkHuman(must(r, `fix-${st}`), `fix ${st}`))))
    result = await check()
  }
  if (!result.complete) halt(`completeness gate gagal setelah ${MAX_COMPLETENESS_ATTEMPTS} putaran. Gap tersisa: ${result.gaps.map((g) => `[${g.stack}] ${g.item}`).join('; ')}`)

  // Commit hasil agar bisa di-merge (squash) nanti
  must(await agent(
    `Di ${wt} jalankan: git add -A && git commit -m "feat(${slug}): implement" dengan baris terakhir pesan "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>". Jika tidak ada perubahan, ok=true tanpa commit. Jangan push.`,
    { label: 'commit-implementation', model: 'haiku', effort: 'low', schema: OK }), 'commit-implementation')
  await progress(`| DONE | completeness gate + commit | IMPLEMENTED |`)

  return { status: 'IMPLEMENTED', worktreePath: wt, planPath: a.planPath, scope, slug, branch, warnings: warnings.length ? warnings : undefined }
}

try {
  return await runBody()
} catch (e) {
  if (e instanceof Halt) return { status: 'NEEDS_HUMAN_INPUT', reason: e.message, worktreePath: wt || null, planPath: a.planPath, warnings: warnings.length ? warnings : undefined }
  throw e
}
