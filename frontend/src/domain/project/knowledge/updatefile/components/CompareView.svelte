<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { diffLines } from './diff';

  let { mine, theirs }: { mine: string; theirs: string } = $props();

  const rows = $derived(diffLines(mine, theirs));

  async function copyMine() {
    const ok = await copyText(mine);
    toast.push({ type: ok ? 'success' : 'error', message: $t(ok ? 'knowledge.copy.done' : 'knowledge.copy.failed') });
  }
</script>

<div class="mt-4 grid gap-3 md:grid-cols-2">
  <div>
    <div class="mb-1 flex items-center justify-between gap-2">
      <h3 class="text-[13px] font-medium">{$t('knowledge.conflict.mine')}</h3>
      <button
        type="button"
        onclick={copyMine}
        class="rounded-base px-2 py-0.5 text-[13px] text-muted hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
      >{$t('knowledge.copy.mine')}</button>
    </div>
    <pre data-testid="compare-mine" class="max-h-72 overflow-auto rounded-base border border-border p-2 font-mono text-xs whitespace-pre-wrap">{mine}</pre>
  </div>
  <div>
    <h3 class="mb-1 text-[13px] font-medium">{$t('knowledge.conflict.theirs')}</h3>
    <pre data-testid="compare-theirs" class="max-h-72 overflow-auto rounded-base border border-border p-2 font-mono text-xs whitespace-pre-wrap">{theirs}</pre>
  </div>
</div>
{#if rows}
  <ul class="mt-3 max-h-56 overflow-auto rounded-base border border-border font-mono text-xs" aria-label={$t('knowledge.conflict.compare')}>
    {#each rows as row, i (i)}
      <li
        data-op={row.op}
        class="px-2 whitespace-pre-wrap {row.op === 'add' ? 'bg-pill-positive-bg text-pill-positive-ink' : row.op === 'del' ? 'bg-pill-negative-bg text-pill-negative-ink' : ''}"
      >
        <span aria-hidden="true">{row.op === 'add' ? '+ ' : row.op === 'del' ? '- ' : '  '}</span>{row.text}
      </li>
    {/each}
  </ul>
{/if}
