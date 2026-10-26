<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { baseName } from '../../base';
  import { MAX_LIMIT, type SearchItem } from '../types';

  let {
    items,
    total,
    limit,
    dim = false,
    onopen,
    onmore,
  }: { items: SearchItem[]; total: number; limit: number; dim?: boolean; onopen: (path: string) => void; onmore: () => void } = $props();
</script>

<div class="flex flex-col gap-2 {dim ? 'opacity-60' : ''}">
  <p class="px-1 text-xs text-muted">{$t('knowledge.search.total', { shown: items.length, total })}</p>
  <ul class="flex flex-col">
    {#each items as it (it.path)}
      <li>
        <button
          type="button"
          onclick={() => onopen(it.path)}
          class="flex w-full flex-col gap-0.5 rounded-base px-2 py-2 text-left hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
        >
          <span class="font-medium break-words">{it.title || baseName(it.path)}</span>
          <span class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <span class="font-mono text-xs break-all text-muted">{it.path}</span>
            {#if it.section}
              <span class="outlined rounded-full bg-raised px-2 py-0.5 text-xs break-all text-muted" aria-label={$t('knowledge.search.section', { section: it.section })}>§ {it.section}</span>
            {/if}
          </span>
          <span class="text-[13px] break-words text-muted">
            {#each it.snippet as s, i (i)}{#if s.highlight}<mark class="rounded-base bg-brand/40 text-fg">{s.text}</mark>{:else}{s.text}{/if}{/each}
          </span>
        </button>
      </li>
    {/each}
  </ul>
  {#if total > items.length && limit < MAX_LIMIT}
    <button
      type="button"
      onclick={onmore}
      class="self-start rounded-base px-2 py-1 text-sm underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
    >{$t('knowledge.search.more')}</button>
  {/if}
</div>
