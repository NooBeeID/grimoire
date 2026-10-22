<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from '../../shared/components/Avatar.svelte';
  import Button from '../../shared/components/Button.svelte';
  import LocaleSwitch from '../../shared/components/LocaleSwitch.svelte';
  import ThemeSwitch from '../../shared/components/ThemeSwitch.svelte';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
  import { logoutAndLeave } from '../boot';
  import { current } from '../routes/router';
  let { children }: { children?: Snippet } = $props();

  let leaving = $state(false);
  let menuOpen = $state(false);
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
    { href: '/', label: $t('nav.projects'), icon: 'folder' },
    { href: '/settings', label: $t('nav.settings'), icon: 'settings' },
    ...($session.identity?.role === 'super_admin'
      ? [
          { href: '/dashboard', label: $t('dashboard.menu'), icon: 'chart' },
          { href: '/admin/users', label: $t('nav.users'), icon: 'users' },
          { href: '/audit-log', label: $t('auditLog.menu'), icon: 'list' },
        ]
      : []),
  ]);

  // Menu ponsel menutup sendiri setelah pindah halaman.
  $effect(() => {
    void path;
    menuOpen = false;
  });
</script>

{#snippet brand()}
  <span class="flex items-center gap-2.5">
    <span class="grid h-7 w-7 place-items-center rounded-card bg-accent text-on-accent shadow-button">
      <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M12 6.5C10 5 7 4.5 4 5v13c3-.5 6 0 8 1.5 2-1.5 5-2 8-1.5V5c-3-.5-6 0-8 1.5Z" />
        <path d="M12 6.5v13" />
      </svg>
    </span>
    <span class="text-[15px] font-semibold tracking-[-0.01em] text-fg">{$t('app.name')}</span>
  </span>
{/snippet}

<a href="#main" data-router-ignore class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2">
  Skip to content
</a>

<div class="flex min-h-screen flex-col bg-ground md:h-screen md:flex-row">
  <!-- Ponsel: top bar 48px (menu + nama produk). Desktop: tidak ada top bar, sidebar memegang semuanya. -->
  <div class="flex h-12 shrink-0 items-center justify-between px-4 md:hidden">
    {@render brand()}
    <button
      type="button"
      aria-expanded={menuOpen}
      aria-controls="app-menu"
      onclick={() => (menuOpen = !menuOpen)}
      class="inline-flex h-8 items-center rounded-base border border-control bg-surface px-3 text-sm font-medium focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
    >
      {$t('nav.menu')}
    </button>
  </div>

  <aside
    id="app-menu"
    class="{menuOpen ? 'flex' : 'hidden'} shrink-0 flex-col gap-6 px-3 pb-3 md:flex md:w-[228px] md:py-4"
  >
    <div class="hidden px-2.5 md:block">{@render brand()}</div>

    <nav aria-label={$t('nav.main')} class="flex flex-col gap-0.5">
      {#each links as l (l.href)}
        {@const active = isActive(l.href)}
        <a
          href={l.href}
          aria-current={active ? 'page' : undefined}
          class="flex h-8 items-center gap-2.5 rounded-base px-2.5 text-sm focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {active
            ? 'border border-border bg-surface font-medium text-fg shadow-xs'
            : 'border border-transparent text-muted hover:bg-surface/60 hover:text-fg'}"
        >
          <svg viewBox="0 0 24 24" width="16" height="16" class={active ? 'text-accent-ink' : ''} fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            {#if l.icon === 'folder'}
              <path d="M3 7.5A1.5 1.5 0 0 1 4.5 6h4l2 2.5h9A1.5 1.5 0 0 1 21 10v8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18Z" />
            {:else if l.icon === 'users'}
              <circle cx="9" cy="8.5" r="3" /><path d="M3.5 19a5.5 5.5 0 0 1 11 0" /><path d="M16 5.8a3 3 0 0 1 0 5.4M17.5 14.3A5.5 5.5 0 0 1 20.5 19" />
            {:else if l.icon === 'chart'}
              <path d="M5 19v-6M12 19V5M19 19v-9" />
            {:else if l.icon === 'settings'}
              <circle cx="12" cy="12" r="3" /><path d="M12 3v2.5M12 18.5V21M4.2 7.5l2.2 1.3M17.6 15.2l2.2 1.3M4.2 16.5l2.2-1.3M17.6 8.8l2.2-1.3" />
            {:else}
              <path d="M5 6h14M5 12h14M5 18h9" />
            {/if}
          </svg>
          {l.label}
        </a>
      {/each}
    </nav>

    <div class="mt-auto flex flex-col gap-2">
      {#if $session.identity}
        <a href="/profile" aria-label={$t('nav.profile')} class="flex items-center gap-2.5 rounded-base px-2.5 py-1.5 hover:bg-surface/60 focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">
          <Avatar name={$session.identity.name} size={28} />
          <span class="min-w-0 truncate text-sm">{$session.identity.name}</span>
        </a>
      {/if}
      <div class="flex items-center gap-2 px-0.5">
        <div class="min-w-0 flex-1"><LocaleSwitch /></div>
        <ThemeSwitch />
        {#if $session.identity}<Button size="sm" variant="secondary" loading={leaving} onclick={leave}>{$t('nav.logout')}</Button>{/if}
      </div>
    </div>
  </aside>

  <main
    id="main"
    class="min-w-0 flex-1 bg-surface p-4 max-md:mt-0 md:mt-2 md:mr-2 md:mb-2 md:overflow-y-auto md:rounded-sheet md:border md:border-border md:p-7 md:shadow-sheet"
  >
    {@render children?.()}
  </main>
</div>
