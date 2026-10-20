<script lang="ts">
  import type { Snippet } from 'svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import { t } from '../../shared/stores/i18n';
  let { children }: { children?: Snippet } = $props();
</script>

<a href="#main" data-router-ignore class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2">
  Skip to content
</a>
<header class="flex items-center justify-between border-b border-border px-4 py-3">
  <span class="font-semibold">{$t('app.name')}</span>
  <LocaleSwitch />
</header>
<main id="main" class="p-6">{@render children?.()}</main>
