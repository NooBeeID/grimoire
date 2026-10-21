<script lang="ts">
  let { method, large = false }: { method: string; large?: boolean } = $props();
</script>

<span
  class="inline-flex shrink-0 items-center justify-center rounded-base bg-pill-neutral-bg font-mono font-semibold text-pill-neutral-ink {large
    ? 'h-7 min-w-16 px-2 text-xs'
    : 'h-5 min-w-12 px-1.5 text-[11px]'}">{method}</span
>
