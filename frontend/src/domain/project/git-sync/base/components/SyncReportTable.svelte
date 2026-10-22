<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { reasonText } from '../reason';
  import type { GitSyncReportItem, ItemStatus, Kind } from '../types';
  import StatusBadge from './StatusBadge.svelte';

  let { items, kind, truncated = false, total }: { items: GitSyncReportItem[]; kind: Kind; truncated?: boolean; total?: number } = $props();

  const ORDER: ItemStatus[] = ['rejected', 'created', 'updated', 'skipped', 'unchanged'];
  const sorted = $derived([...items].sort((a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status) || a.repo_path.localeCompare(b.repo_path)));
  let open = $state<Record<string, boolean>>({});

  const targetOf = (i: GitSyncReportItem) => i.target;

  async function copyReason(i: GitSyncReportItem) {
    const ok = await copyText(`${i.repo_path}: ${reasonText($t, kind, i)}`);
    toast.push({ type: ok ? 'success' : 'error', message: $t(ok ? 'gitSync.report.copied' : 'knowledge.copy.failed') });
  }
</script>

{#if items.length === 0}
  <p class="rounded-card border border-border p-6 text-center text-sm text-muted">{$t('gitSync.report.empty')}</p>
{:else}
  <div class="max-h-[360px] overflow-auto rounded-card border border-border bg-surface shadow-panel" data-testid="report-table">
    <table class="w-full border-collapse text-left text-sm">
      <thead class="sticky top-0 z-10">
        <tr class="bg-raised">
          <th scope="col" class="h-10 px-3 text-[13px] font-medium text-muted">{$t('gitSync.report.repoPath')}</th>
          <th scope="col" class="h-10 px-3 text-[13px] font-medium text-muted max-sm:hidden">{$t('gitSync.report.target')}</th>
          <th scope="col" class="h-10 px-3 text-[13px] font-medium text-muted">{$t('gitSync.report.status')}</th>
          <th scope="col" class="h-10 px-3 text-[13px] font-medium text-muted max-sm:hidden">{$t('gitSync.report.reason')}</th>
        </tr>
      </thead>
      <tbody>
        {#each sorted as i (i.repo_path)}
          {@const expandable = i.status === 'rejected' && i.issues.length > 0}
          <tr class="min-h-12 border-t border-rule align-top">
            <td class="max-w-[16rem] px-3 py-3">
              <div class="flex items-start gap-1">
                {#if expandable}
                  <button
                    type="button"
                    aria-expanded={open[i.repo_path] ? 'true' : 'false'}
                    aria-label={$t('gitSync.report.expand')}
                    onclick={() => (open[i.repo_path] = !open[i.repo_path])}
                    class="mt-0.5 shrink-0 rounded-base p-0.5 text-muted transition-colors hover:text-fg focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
                  >
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class={open[i.repo_path] ? 'rotate-90' : ''}>
                      <path d="M6 3l5 5-5 5" />
                    </svg>
                  </button>
                {/if}
                <code class="min-w-0 font-mono text-[13px] break-all">{i.repo_path}</code>
              </div>
              <p class="mt-1 text-[13px] text-muted sm:hidden">
                {#if targetOf(i)}<code class="font-mono">{targetOf(i)}</code> · {/if}{reasonText($t, kind, i)}
              </p>
            </td>
            <td class="px-3 py-3 max-sm:hidden">
              {#if i.target}
                <code class="font-mono text-[13px] break-all">{i.target}</code>
                {#if i.endpoint_count}<span class="text-[13px] text-muted"> · {$t('gitSync.report.endpoints', { n: i.endpoint_count })}</span>{/if}
              {:else}
                <span class="text-muted">–</span>
              {/if}
            </td>
            <td class="px-3 py-3"><StatusBadge status={i.status} /></td>
            <td class="px-3 py-3 max-sm:hidden">
              {#if i.status === 'created' || i.status === 'updated' || i.status === 'unchanged'}
                <span class="text-muted">–</span>
              {:else}
                <span class="flex items-start gap-1">
                  <span class="min-w-0 text-[13px]">{reasonText($t, kind, i)}</span>
                  <button
                    type="button"
                    aria-label={$t('gitSync.report.copyReason')}
                    onclick={() => copyReason(i)}
                    class="shrink-0 rounded-base p-0.5 text-muted transition-colors hover:text-fg focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
                  >
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true">
                      <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
                      <path d="M10.5 5.5v-2a1.5 1.5 0 0 0-1.5-1.5H4A1.5 1.5 0 0 0 2.5 3.5V9A1.5 1.5 0 0 0 4 10.5h1.5" />
                    </svg>
                  </button>
                </span>
              {/if}
            </td>
          </tr>
          {#if expandable && open[i.repo_path]}
            <tr class="bg-raised">
              <td colspan="4" class="px-3 py-2">
                <p class="mb-1 text-xs text-muted">{$t('gitSync.report.issues')}</p>
                <ul class="flex flex-col gap-1 text-[13px]">
                  {#each i.issues as is, n (n)}
                    <li class="flex flex-wrap gap-x-2">
                      <code class="font-mono">{is.code}</code>
                      <span>{is.message}</span>
                      <code class="font-mono text-muted">{is.pointer}</code>
                      {#if is.line > 0}<span class="text-muted tabular-nums">:{is.line}</span>{/if}
                    </li>
                  {/each}
                </ul>
              </td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
    {#if truncated}
      <p class="border-t border-rule px-3 py-2 text-[13px] text-muted">{$t('gitSync.report.truncated', { n: items.length, total: total ?? items.length })}</p>
    {/if}
  </div>
{/if}
