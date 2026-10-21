<script lang="ts">
  import type { Snippet } from 'svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import ThemeSwitch from '../../shared/components/ThemeSwitch.svelte';
  import { t } from '../../shared/stores/i18n';
  let { children }: { children?: Snippet } = $props();
</script>

<main id="main" class="relative grid min-h-dvh place-items-center bg-ground p-4">
  <div class="absolute top-4 right-4 flex items-center gap-2"><div class="w-28"><LocaleSwitch /></div><ThemeSwitch /></div>
  <div class="flex w-full max-w-md flex-col items-center gap-5">
    <span class="flex items-center gap-2.5">
      <span class="grid h-9 w-9 place-items-center rounded-card bg-accent text-on-accent shadow-button">
        <svg viewBox="0 0 24 24" width="21" height="21" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 6.5C10 5 7 4.5 4 5v13c3-.5 6 0 8 1.5 2-1.5 5-2 8-1.5V5c-3-.5-6 0-8 1.5Z" />
          <path d="M12 6.5v13" />
        </svg>
      </span>
      <span class="text-lg font-semibold tracking-[-0.01em]">{$t('app.name')}</span>
    </span>
    <div class="w-full rounded-sheet border border-border bg-surface p-8 shadow-sheet">
      {@render children?.()}
    </div>
  </div>
</main>
