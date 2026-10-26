<script lang="ts">
  import type { Snippet } from 'svelte';

  let { tone = 'neutral', children }: { tone?: 'neutral' | 'warning' | 'positive' | 'negative'; children: Snippet } = $props();
  const cls = {
    neutral: 'bg-pill-neutral-bg text-pill-neutral-ink',
    warning: 'bg-pill-warning-bg text-pill-warning-ink',
    positive: 'bg-pill-positive-bg text-pill-positive-ink',
    negative: 'bg-pill-negative-bg text-pill-negative-ink',
  };
</script>

<span class="inline-flex h-5 shrink-0 items-center rounded-base px-2 text-xs font-medium whitespace-nowrap {cls[tone]}">{@render children()}</span>
