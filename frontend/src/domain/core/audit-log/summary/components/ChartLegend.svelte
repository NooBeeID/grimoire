<script lang="ts">
  import type { LegendItem } from './chart/legend';
  let { items }: { items: LegendItem[] } = $props();
  const uid = $props.id();
</script>

<ul class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
  {#each items as it, i (it.label)}
    <li class="inline-flex items-center gap-1.5">
      <svg width="14" height="10" viewBox="0 0 14 10" aria-hidden="true">
        {#if it.kind === 'line'}
          <line x1="0" y1="5" x2="14" y2="5" stroke={it.color} stroke-width="2" />
        {:else if it.kind === 'hatch'}
          <defs>
            <pattern id="hatch-{uid}-{i}" width="6" height="6" patternUnits="userSpaceOnUse">
              <path d="M0 6L6 0" stroke={it.color} stroke-width="1.5" />
            </pattern>
          </defs>
          <rect width="14" height="10" fill="url(#hatch-{uid}-{i})" stroke={it.color} stroke-width="1" />
        {:else}
          <rect width="14" height="10" fill={it.color} />
        {/if}
      </svg>
      {it.label}
    </li>
  {/each}
</ul>
