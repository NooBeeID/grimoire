<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, meta, flush = false, level = 4, children }: { title: string; meta?: Snippet; flush?: boolean; level?: 3 | 4; children: Snippet } = $props();
</script>

<section class="overflow-hidden rounded-card border border-border bg-surface shadow-panel">
  <header class="flex min-h-11 items-center justify-between gap-3 bg-raised px-4 py-2">
    <svelte:element this={level === 3 ? 'h3' : 'h4'} class="text-sm font-semibold">{title}</svelte:element>
    {#if meta}<div class="flex flex-wrap items-center gap-2 text-xs text-muted">{@render meta()}</div>{/if}
  </header>
  <div class={flush ? '' : 'p-4'}>{@render children()}</div>
</section>
