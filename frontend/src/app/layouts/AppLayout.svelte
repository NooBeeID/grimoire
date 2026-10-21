<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../../shared/components/Button.svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
  import { logoutAndLeave } from '../boot';
  import { current } from '../routes/router';
  let { children }: { children?: Snippet } = $props();

  let leaving = $state(false);
  async function leave() {
    leaving = true;
    try {
      await logoutAndLeave();
    } finally {
      leaving = false;
    }
  }
  const path = $derived($current.location.path);
  const isActive = (href: string) => (href === '/' ? path === '/' || path.startsWith('/projects/') : path === href);
  const links = $derived([
    { href: '/', label: $t('nav.projects') },
    ...($session.identity?.role === 'super_admin'
      ? [
          { href: '/admin/users', label: $t('nav.users') },
          { href: '/audit-log', label: $t('auditLog.menu') },
        ]
      : []),
  ]);
</script>

<a href="#main" data-router-ignore class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2">
  Skip to content
</a>
<header class="flex items-center justify-between gap-4 border-b border-border px-4 py-3">
  <div class="flex items-center gap-6">
    <span class="font-semibold">{$t('app.name')}</span>
    <nav aria-label={$t('nav.main')} class="flex gap-4">
      {#each links as l (l.href)}
        <a href={l.href} aria-current={isActive(l.href) ? 'page' : undefined} class="hover:underline {isActive(l.href) ? 'font-semibold' : ''}">{l.label}</a>
      {/each}
    </nav>
  </div>
  <div class="flex items-center gap-3">
    {#if $session.identity}
      <span class="text-sm">{$session.identity.name}</span>
      <a href="/profile" class="text-sm hover:underline">{$t('nav.profile')}</a>
      <Button size="sm" variant="secondary" loading={leaving} onclick={leave}>{$t('nav.logout')}</Button>
    {/if}
    <LocaleSwitch />
  </div>
</header>
<main id="main" class="p-6">{@render children?.()}</main>
