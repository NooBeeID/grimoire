<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let { path, ondir, oncopy }: { path: string; ondir: (path: string) => void; oncopy: () => void } = $props();
  const segs = $derived(path.split('/'));
</script>

<div class="flex flex-wrap items-center gap-2">
  <nav aria-label={$t('knowledge.breadcrumb.label')}>
    <ol class="flex flex-wrap items-center gap-1 font-mono text-[13px]">
      {#each segs as seg, i (i)}
        <li class="flex items-center gap-1">
          {#if i < segs.length - 1}
            <button
              type="button"
              onclick={() => ondir(segs.slice(0, i + 1).join('/'))}
              class="rounded-base px-1 text-muted underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
            >{seg}</button>
            <span class="text-muted" aria-hidden="true">/</span>
          {:else}
            <span aria-current="page" class="px-1 break-all">{seg}</span>
          {/if}
        </li>
      {/each}
    </ol>
  </nav>
  <button
    type="button"
    onclick={oncopy}
    class="rounded-base px-2 py-0.5 text-[13px] text-muted hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
  >{$t('knowledge.actions.copyPath')}</button>
</div>
