<script lang="ts">
  import type { Snippet } from 'svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  let { children }: { children?: Snippet } = $props();
</script>

<main id="main" class="relative min-h-dvh p-6">
  <div class="absolute top-4 right-4"><LocaleSwitch /></div>
  {@render children?.()}
</main>
