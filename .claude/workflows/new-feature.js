export const meta = {
  name: 'new-feature',
  description: 'Implementasi plan lokal di worktree (implement-feature), verify loop (test), lalu squash-merge opsional bila targetBranch diberikan',
  whenToUse: 'Setelah plan-from-usecase menghasilkan plan. Worktree dibiarkan utuh bila gagal',
  phases: [
    { title: 'Resolve', detail: 'validasi planPath, hitung slug' },
    { title: 'Merge', detail: 'squash-merge ke targetBranch (opsional, tanpa push)' },
  ],
}

// args: { planPath, scope?, maxAttempts=3, minCoverage?, slug?, base?, notes?, maxAgentParalel?, targetBranch? }

class Halt extends Error {}
const halt = (reason) => { throw new Halt(reason) }

const a = args || {}

function slugFromPlanPath(p) {
  const parts = String(p).split('/').filter(Boolean)
  const file = parts.pop() || ''
  const stripDate = (s) => s.replace(/^\d{4}-\d{2}-\d{2}-/, '')
  let name = stripDate(file.replace(/\.md$/i, ''))
  if (['overview', 'plan', 'index'].includes(name.toLowerCase())) name = stripDate(parts.pop() || name)
  return name.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/-+/g, '-').replace(/^-+|-+$/g, '')
}

if (!a.planPath) return { status: 'NEEDS_HUMAN_INPUT', reason: 'planPath wajib diisi' }

const maxAttempts = Number.isInteger(a.maxAttempts) && a.maxAttempts > 0 ? a.maxAttempts : 3
const slug = a.slug || slugFromPlanPath(a.planPath)
if (!slug) return { status: 'NEEDS_HUMAN_INPUT', reason: `slug tidak bisa diturunkan dari planPath: ${a.planPath}` }

try {
  // ---- 1. Resolve
  phase('Resolve')
  const resolved = await agent(
    `Periksa path lokal ${a.planPath} (jangan fetch URL). Jika berupa file, resolvedPath = path itu. Jika berupa direktori, resolvedPath = <dir>/overview.md bila ada, kalau tidak <dir>/plan.md. exists=false bila tidak ada atau kosong. Jangan mengubah apa pun.`,
    {
      label: 'resolve-plan', model: 'haiku', effort: 'low',
      schema: { type: 'object', properties: { exists: { type: 'boolean' }, resolvedPath: { type: 'string' } }, required: ['exists'] },
    })
  if (resolved == null) halt('agent resolve-plan gagal (kegagalan infrastruktur)')
  if (!resolved.exists) halt(`plan tidak ditemukan: ${a.planPath}`)
  const planPath = resolved.resolvedPath || a.planPath

  // ---- 2. implement-feature (child workflow, flat)
  const impl = await workflow('implement-feature', {
    planPath, scope: a.scope, slug, base: a.base, notes: a.notes, maxAgentParalel: a.maxAgentParalel,
  })
  if (impl == null || impl.status !== 'IMPLEMENTED') {
    return { ...(impl || { status: 'NEEDS_HUMAN_INPUT', reason: 'implement-feature gagal tanpa hasil' }), planPath }
  }

  // ---- 3. test (child workflow, flat)
  const result = await workflow('test', {
    planPath, worktreePath: impl.worktreePath, scope: impl.scope, maxAttempts, slug, minCoverage: a.minCoverage,
  })
  if (result == null) return { status: 'NEEDS_HUMAN_INPUT', reason: 'test workflow gagal tanpa hasil', planPath, worktreePath: impl.worktreePath }

  // ---- 4. Gagal -> worktree dan branch dibiarkan utuh
  if (result.status !== 'DONE') return { ...result, planPath, worktreePath: impl.worktreePath, branch: `feat/${slug}` }

  // ---- 5. Merge (hanya bila targetBranch diberikan)
  if (!a.targetBranch) return { ...result, planPath, worktreePath: impl.worktreePath, branch: `feat/${slug}`, warnings: impl.warnings }

  phase('Merge')
  const branch = `feat/${slug}`
  const merge = await agent(
    `Squash-merge branch ${branch} ke ${a.targetBranch} TANPA push. Worktree feature: ${impl.worktreePath}. Checkout utama: repo root (bukan worktree). Langkah:
1. Di worktree feature: bila ada perubahan belum ter-commit, git add -A && git commit -m "feat(${slug}): finalize".
2. Di checkout utama: catat branch saat ini. Jika ada perubahan pada file TRACKED (git status --porcelain tanpa baris "??"), berhenti: status="blocked", jelaskan di reason. Jangan stash atau reset.
3. git checkout ${a.targetBranch} (harus sudah ada; kalau tidak, status="blocked").
4. git merge --squash ${branch}. Jika konflik: git merge --abort (atau git reset --merge bila abort tidak tersedia), kembali ke branch semula, status="conflict", daftar file konflik di reason. Worktree dan branch JANGAN dihapus.
5. Bila sukses: git commit -m "feat(${slug}): <ringkasan satu baris dari plan ${planPath}>" dengan baris terakhir pesan "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>".
6. Lalu git worktree remove ${impl.worktreePath} dan git branch -D ${branch}. Jika langkah ini gagal, tetap status="merged" dan catat di warnings.
7. Jika branch semula bukan ${a.targetBranch}, checkout kembali ke branch semula.
Jangan git push.`,
    {
      label: 'squash-merge', model: 'sonnet',
      schema: {
        type: 'object',
        properties: {
          status: { type: 'string', enum: ['merged', 'conflict', 'blocked'] },
          reason: { type: 'string' },
          warnings: { type: 'array', items: { type: 'string' } },
        },
        required: ['status'],
      },
    })
  if (merge == null) halt('agent squash-merge gagal (kegagalan infrastruktur); worktree dan branch dibiarkan utuh')
  if (merge.status !== 'merged') {
    return { status: 'NEEDS_HUMAN_INPUT', reason: `merge ${merge.status}: ${merge.reason || ''}`, planPath, worktreePath: impl.worktreePath, branch }
  }
  const allWarnings = [...(impl.warnings || []), ...(merge.warnings || [])]
  return { ...result, planPath, mergedToBranch: a.targetBranch, warnings: allWarnings.length ? allWarnings : undefined }
} catch (e) {
  if (e instanceof Halt) return { status: 'NEEDS_HUMAN_INPUT', reason: e.message, planPath: a.planPath }
  throw e
}
