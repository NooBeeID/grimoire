<script lang="ts">
  import type { Snippet } from 'svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import { session } from '../../shared/stores/session';
  import { t } from '../../shared/stores/i18n';
  import { current } from '../routes/router';
  let { children }: { children?: Snippet } = $props();
</script>

<a href="#main" data-router-ignore class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2">
  Skip to content
</a>
<header class="flex items-center justify-between border-b border-border px-4 py-3">
  <span class="font-semibold">{$t('app.name')}</span>
  <nav aria-label={$t('nav.main')}>
    {#if $session.identity?.role === 'super_admin'}
      <a
        href="/audit-log"
        aria-current={$current.location.path.startsWith('/audit-log') ? 'page' : undefined}
        class="text-sm font-medium hover:underline">{$t('auditLog.menu')}</a
      >
    {/if}
  </nav>
  <LocaleSwitch />
</header>
<main id="main" class="p-6">{@render children?.()}</main>
