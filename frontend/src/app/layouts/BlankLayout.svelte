<script lang="ts">
  import type { Snippet } from 'svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import Mascot from '../../shared/components/Mascot.svelte';
  import ThemeSwitch from '../../shared/components/ThemeSwitch.svelte';
  import { t } from '../../shared/stores/i18n';
  let { children }: { children?: Snippet } = $props();
</script>

<main id="main" class="relative grid min-h-dvh place-items-center p-4">
  <div class="absolute top-4 right-4 z-10 flex items-center gap-2"><div class="w-28"><LocaleSwitch /></div><ThemeSwitch /></div>

  <div class="flex w-full max-w-sm flex-col gap-6">
    <div class="flex items-center gap-2.5">
      <Mascot size={28} label={$t('app.name')} />
      <span class="text-lg font-semibold tracking-[-0.02em]">{$t('app.name')}</span>
    </div>
    <div class="glass w-full rounded-sheet p-8">
      {@render children?.()}
    </div>
  </div>
</main>
