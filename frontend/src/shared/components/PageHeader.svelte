<script lang="ts">
  import type { Snippet } from 'svelte';

  // Title row layar kerja: judul 22px (satu h1 per layar), kalimat pendek tentang data, dan aksi nyata di kanan.
  let {
    title,
    description,
    testid,
    action,
  }: { title: string; description?: string; testid?: string; action?: Snippet } = $props();
</script>

<header class="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
  <div class="min-w-0">
    <h1 data-testid={testid} class="text-[22px] leading-8 font-semibold break-words">{title}</h1>
    {#if description}<p class="mt-1 text-sm text-muted">{description}</p>{/if}
  </div>
  {#if action}<div class="flex shrink-0 items-center gap-2">{@render action()}</div>{/if}
</header>
