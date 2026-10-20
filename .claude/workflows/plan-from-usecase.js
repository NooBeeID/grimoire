export const meta = {
  name: 'plan-from-usecase',
  description: 'Pecah goal/ticket menjadi usecase, plan tiap usecase paralel, simpan sebagai file lokal di docs/superpowers/plans',
  whenToUse: 'Sebelum new-feature: menghasilkan plan (tunggal atau terdekomposisi) dari goal teks atau file tiket',
  phases: [{ title: 'Planning', detail: 'decompose -> overview -> plan per usecase (paralel) -> frontend -> api-design -> commit' }],
}

// args: { goal, scope: 'backend'|'frontend'|'fullstack', domain, module, service, feature, notes?, images?, prd?, ticket?, commit? }

class Halt extends Error {}
const halt = (reason) => { throw new Halt(reason) }
const must = (v, what) => { if (v == null) halt(`Agent ${what} gagal (hasil null, kegagalan infrastruktur)`); return v }

const a = args || {}
const SCOPES = ['backend', 'frontend', 'fullstack']
const PLAN_ROOT = 'docs/superpowers/plans'

const kebab = (s) => String(s).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')

if (!a.feature || !(a.goal || a.ticket || a.prd)) {
  return { status: 'NEEDS_HUMAN_INPUT', reason: 'feature dan salah satu dari goal/ticket/prd wajib diisi' }
}
if (a.scope && !SCOPES.includes(a.scope)) {
  return { status: 'NEEDS_HUMAN_INPUT', reason: `scope tidak valid: ${a.scope} (backend|frontend|fullstack)` }
}

const scope = a.scope || 'fullstack'
const slug = kebab(a.feature)
const planFile = `${PLAN_ROOT}/${slug}.md`
const planDir = `${PLAN_ROOT}/${slug}`
const overviewFile = `${planDir}/overview.md`
const frontendFile = `${planDir}/frontend-plan.md`
const domain = a.domain || '(turunkan dari ticket)'
const module = a.module || '(turunkan dari ticket)'
const hasBackend = scope === 'backend' || scope === 'fullstack'
const hasFrontend = scope === 'frontend' || scope === 'fullstack'

const BRIEF = [
  `Feature: ${a.feature}`,
  `Scope: ${scope}`,
  `Domain: ${domain}`,
  `Module: ${module}`,
  a.service ? `Service: ${a.service}` : null,
  a.ticket ? `Ticket (baca file ini lebih dulu): ${a.ticket}` : null,
  a.goal ? `Goal:\n${a.goal}` : null,
  a.prd ? `PRD:\n${a.prd}` : null,
  a.notes ? `Catatan:\n${a.notes}` : null,
  a.images && a.images.length ? `Gambar referensi (baca dengan Read):\n${a.images.map((i) => `- ${i}`).join('\n')}` : null,
].filter(Boolean).join('\n')

const CONTEXT_LOADING = `Muat konteks dengan urutan ini sebelum merancang:
1. CLAUDE.md (jika ada)
2. file rules di .claude/rules/ (jika ada)
3. docs/tickets/README.md (konvensi arsitektur, katalog endpoint bus, katalog audit) dan docs/superpowers/specs/*knowledge-hub-design.md
4. knowledge doc module terkait (docs/ atau README di folder module, jika ada)`

const WRITE_FILE_NOTE = `PENULISAN FILE: tulis file hanya dengan Write tool. Jangan memakai heredoc atau echo (pernah gagal diam-diam dan plan hilang). Setelah menulis, Read ulang file itu untuk memastikan file ada dan isinya lengkap.`

const NO_STOPPING_NOTE = `Selesaikan semuanya dalam satu turn, jangan berhenti untuk bertanya soal proses. Gunakan needsHumanInput=true hanya untuk ambiguitas requirement yang nyata (bukan pertanyaan proses), dan isi reason.`

const PLAN_FORMAT = `FORMAT PLAN (terstruktur, semua bagian wajib):
- Metadata: tabel dengan Endpoint, Method, Version (dan Scope/Domain/Module)
- Summary Plan
- Plan: langkah berurutan yang konkret (file yang dibuat/diubah, kontrak bus)
- Migration: file migrasi (nama YYYYMMDDHHMMSSmmm_*.sql) atau "tidak ada"
- Changes: daftar perubahan per file/folder
- Test Scenario: unit/integration test
- E2E Scenarios: kasus positif dan negatif
- Request/Response body: contoh JSON lengkap termasuk format error`

const PLAN_RESULT = {
  type: 'object',
  properties: {
    path: { type: 'string' },
    endpointsChanged: { type: 'boolean' },
    needsHumanInput: { type: 'boolean' },
    reason: { type: 'string' },
  },
  required: ['path', 'endpointsChanged'],
}

const runBody = async () => {
  phase('Planning')
  const usecaseResults = [] // {slug, planPath, ok}
  let planPath = planFile
  let isDecomposed = false
  let endpointsChanged = false

  // ---- 1. decompose-backend
  let decomp = null
  if (hasBackend) {
    decomp = must(await agent(
      `${CONTEXT_LOADING}\n\n${BRIEF}\n\nPecah feature ini menjadi usecase backend. Satu usecase = satu folder feature di dalam module (init.go, handler.go, params.go, usecase.go, repository.go). Jangan memecah artifisial: jika feature kecil, kembalikan tepat 1 usecase. Tentukan resourceRootPath, yaitu folder module (default backend/internal/domain/<domain>/internal/<module>). sharedDesignNotes berisi entity, kontrak bus, tabel, dan keputusan lintas usecase.`,
      {
        label: 'decompose-backend', model: 'opus', agentType: 'backend-architect',
        schema: {
          type: 'object',
          properties: {
            resourceRootPath: { type: 'string' },
            sharedDesignNotes: { type: 'string' },
            usecases: { type: 'array', items: { type: 'object', properties: { slug: { type: 'string' }, description: { type: 'string' } }, required: ['slug', 'description'] } },
          },
          required: ['resourceRootPath', 'sharedDesignNotes', 'usecases'],
        },
      }), 'decompose-backend')
    if (!decomp.usecases.length) halt('decompose-backend tidak menghasilkan usecase; requirement kurang jelas')
    decomp.usecases = decomp.usecases.map((u) => ({ ...u, slug: kebab(u.slug) }))
    isDecomposed = decomp.usecases.length > 1
  }

  // ---- 2. backend plan
  if (hasBackend && isDecomposed) {
    const foundation = must(await agent(
      `${CONTEXT_LOADING}\n\n${BRIEF}\n\nDesain bersama dari dekomposisi:\n${decomp.sharedDesignNotes}\nUsecase: ${JSON.stringify(decomp.usecases)}\nResource root: ${decomp.resourceRootPath}\n\nTulis FOUNDATION plan ke ${overviewFile}. Isinya: overview feature, entity/base yang dipakai bersama (folder base/), kontrak bus dan konstanta endpoint (lib/constants/bus.go, lib/contracts), migrasi, urutan implementasi, dan daftar usecase dengan slug-nya. Plan per usecase ditulis agent lain, jangan tulis di sini.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}\nKembalikan overviewMarkdown berisi isi lengkap file tersebut.`,
      {
        label: 'foundation-overview', model: 'opus', agentType: 'backend-architect',
        schema: {
          type: 'object',
          properties: { overviewPath: { type: 'string' }, overviewMarkdown: { type: 'string' }, endpointsChanged: { type: 'boolean' }, needsHumanInput: { type: 'boolean' }, reason: { type: 'string' } },
          required: ['overviewPath', 'overviewMarkdown'],
        },
      }), 'foundation-overview')
    if (foundation.needsHumanInput) halt(foundation.reason || 'foundation butuh input manusia')
    planPath = foundation.overviewPath || overviewFile
    endpointsChanged = !!foundation.endpointsChanged

    const ucFile = (u) => `${planDir}/usecase-${u.slug}.md`
    const planUsecase = (u) => agent(
      `${CONTEXT_LOADING}\n\n${BRIEF}\n\nKamu hanya merencanakan SATU usecase: "${u.slug}" - ${u.description}\nResource root: ${decomp.resourceRootPath}\n\nOVERVIEW (read-only, konteks bersama; jangan diubah):\n${foundation.overviewMarkdown}\n\nHANYA tulis file ${ucFile(u)}. Jangan menulis atau mengubah file lain, termasuk plan usecase lain.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}`,
      {
        label: `plan-usecase-${u.slug}`, model: 'sonnet', agentType: 'backend-architect',
        schema: PLAN_RESULT,
      })

    const results = await parallel(decomp.usecases.map((u) => () => planUsecase(u)))
    decomp.usecases.forEach((u, i) => {
      if (results[i] && results[i].needsHumanInput) halt(results[i].reason || `usecase ${u.slug} butuh input manusia`)
      if (results[i] && results[i].endpointsChanged) endpointsChanged = true
    })

    const verify = async (list) => must(await agent(
      `Cek file berikut benar-benar ada di disk dan tidak kosong (gunakan ls -l atau Read; jangan menulis apa pun):\n${list.map((u) => `- ${ucFile(u)}`).join('\n')}\nKembalikan satu entri per file dengan path persis seperti ditulis di daftar di atas.`,
      {
        label: 'verify-usecase-files', model: 'haiku', effort: 'low',
        schema: {
          type: 'object',
          properties: { files: { type: 'array', items: { type: 'object', properties: { path: { type: 'string' }, ok: { type: 'boolean' } }, required: ['path', 'ok'] } } },
          required: ['files'],
        },
      }), 'verify-usecase-files')

    let state = await verify(decomp.usecases)
    const isOk = (st, u) => !!st.files.find((f) => f.ok && String(f.path).endsWith(ucFile(u)))
    let missing = decomp.usecases.filter((u) => !isOk(state, u))
    if (missing.length) {
      log(`File plan hilang: ${missing.map((u) => u.slug).join(', ')}. Retry sekali.`)
      const retried = await parallel(missing.map((u) => () => planUsecase(u)))
      missing.forEach((u, i) => { if (retried[i] && retried[i].endpointsChanged) endpointsChanged = true })
      state = await verify(decomp.usecases)
    }
    for (const u of decomp.usecases) {
      usecaseResults.push({ slug: u.slug, planPath: ucFile(u), ok: isOk(state, u) })
    }
    if (!usecaseResults.some((u) => u.ok)) halt('semua plan usecase gagal ditulis')
    const bad = usecaseResults.filter((u) => !u.ok)
    if (bad.length) log(`Plan tetap hilang setelah retry (ok:false): ${bad.map((u) => u.slug).join(', ')}`)
  } else if (hasBackend) {
    const brainstorm = must(await agent(
      `${CONTEXT_LOADING}\n\n${BRIEF}\n\nDekomposisi: ${JSON.stringify(decomp.usecases)}\nCatatan desain: ${decomp.sharedDesignNotes}\n\nBrainstorm pendekatan terbaik untuk feature ini: opsi, trade-off, keputusan, risiko. Jangan menulis file. ${NO_STOPPING_NOTE}`,
      {
        label: 'brainstorm', model: 'opus', agentType: 'backend-architect',
        schema: {
          type: 'object',
          properties: { decision: { type: 'string' }, risks: { type: 'string' }, needsHumanInput: { type: 'boolean' }, reason: { type: 'string' } },
          required: ['decision'],
        },
      }), 'brainstorm')
    if (brainstorm.needsHumanInput) halt(brainstorm.reason || 'brainstorm butuh input manusia')
    const written = must(await agent(
      `${CONTEXT_LOADING}\n\n${BRIEF}\n\nResource root: ${decomp.resourceRootPath}\nKeputusan brainstorm:\n${brainstorm.decision}\nRisiko:\n${brainstorm.risks || '-'}\n\nTulis plan tunggal ke ${planFile}.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}`,
      { label: 'write-plan', model: 'opus', agentType: 'backend-architect', schema: PLAN_RESULT }), 'write-plan')
    if (written.needsHumanInput) halt(written.reason || 'write-plan butuh input manusia')
    planPath = written.path || planFile
    endpointsChanged = !!written.endpointsChanged
    usecaseResults.push({ slug: decomp.usecases[0].slug, planPath, ok: true })
  }

  // ---- 3. frontend
  if (hasFrontend) {
    if (scope === 'frontend') {
      const fe = must(await agent(
        `${CONTEXT_LOADING}\n\n${BRIEF}\n\nTulis plan frontend tunggal ke ${planFile}. Kontrak API yang sudah ada ada di api/modules/**.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}`,
        { label: 'frontend-plan', model: 'opus', agentType: 'frontend-architect', schema: PLAN_RESULT }), 'frontend-plan')
      if (fe.needsHumanInput) halt(fe.reason || 'frontend plan butuh input manusia')
      planPath = fe.path || planFile
      endpointsChanged = !!fe.endpointsChanged
      usecaseResults.push({ slug, planPath, ok: true })
    } else if (isDecomposed) {
      // FE tidak dipecah per usecase
      const fe = must(await agent(
        `${CONTEXT_LOADING}\n\n${BRIEF}\n\nBaca ${overviewFile} dan semua ${planDir}/usecase-*.md (kontrak API/request/response ada di sana). Tulis SATU plan frontend ke ${frontendFile} yang mencakup seluruh feature. Frontend tidak dipecah per usecase.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}`,
        { label: 'frontend-plan', model: 'opus', agentType: 'frontend-architect', schema: PLAN_RESULT }), 'frontend-plan')
      if (fe.needsHumanInput) halt(fe.reason || 'frontend plan butuh input manusia')
      if (fe.endpointsChanged) endpointsChanged = true
    } else {
      const fe = must(await agent(
        `${CONTEXT_LOADING}\n\n${BRIEF}\n\nBaca plan backend di ${planPath}. APPEND bagian "## Frontend Plan" di akhir file itu (jangan mengubah bagian backend). Baca dulu isi file, tambahkan di akhir dengan Edit/Write, lalu Read ulang untuk memastikan bagian backend masih utuh.\n${PLAN_FORMAT}\n${WRITE_FILE_NOTE}\n${NO_STOPPING_NOTE}`,
        { label: 'frontend-plan-append', model: 'opus', agentType: 'frontend-architect', schema: PLAN_RESULT }), 'frontend-plan-append')
      if (fe.needsHumanInput) halt(fe.reason || 'frontend plan butuh input manusia')
      if (fe.endpointsChanged) endpointsChanged = true
    }
  }

  // ---- 4. api-design
  if (endpointsChanged) {
    must(await agent(
      `Baca plan di ${planPath}${isDecomposed ? ` dan ${planDir}/*.md` : ''}. Perbarui spesifikasi di api/modules/<domain>/<module>.yml untuk endpoint yang berubah/baru, lalu jalankan make api-bundle. Jangan mengedit api/openapi.yml manual. Jika make target belum ada, laporkan bundled=false.`,
      {
        label: 'api-design', model: 'sonnet', agentType: 'api-design',
        schema: { type: 'object', properties: { updated: { type: 'array', items: { type: 'string' } }, bundled: { type: 'boolean' } }, required: ['updated', 'bundled'] },
      }), 'api-design')
  }

  // ---- 5. commit (worktree di-fork dari HEAD, plan harus ikut tersedia)
  let committed = false
  if (a.commit !== false) {
    const c = must(await agent(
      `Commit plan supaya ikut terbawa saat worktree di-fork dari HEAD. Jalankan: git add -f ${isDecomposed ? planDir : planFile}${endpointsChanged ? ' api/' : ''} lalu git commit -m "docs(plan): ${slug}" dengan baris terakhir pesan "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>". Hanya file itu; jangan menambahkan file lain. Jika tidak ada perubahan, committed=false. Jangan push.`,
      { label: 'commit-plan', model: 'haiku', effort: 'low', schema: { type: 'object', properties: { committed: { type: 'boolean' }, note: { type: 'string' } }, required: ['committed'] } }), 'commit-plan')
    committed = c.committed
  }

  return {
    status: 'DONE',
    planPath,
    planDir: isDecomposed ? planDir : PLAN_ROOT,
    slug,
    scope,
    endpointsChanged,
    isDecomposed,
    committed,
    usecases: usecaseResults,
  }
}

try {
  return await runBody()
} catch (e) {
  if (e instanceof Halt) return { status: 'NEEDS_HUMAN_INPUT', reason: e.message, slug, scope }
  throw e
}
