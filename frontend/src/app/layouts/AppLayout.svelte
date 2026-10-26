<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from '../../shared/components/Avatar.svelte';
  import Button from '../../shared/components/Button.svelte';
  import Mascot from '../../shared/components/Mascot.svelte';
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
  const isAdmin = $derived($session.identity?.role === 'super_admin');
  const links = $derived([
    ...(isAdmin ? [{ href: '/dashboard', label: $t('dashboard.menu'), icon: 'chart' }] : []),
    { href: '/', label: $t('nav.projects'), icon: 'folder' },
    ...(isAdmin ? [{ href: '/admin/users', label: $t('nav.users'), icon: 'users' }] : []),
    { href: '/settings', label: $t('nav.settings'), icon: 'settings' },
    ...(isAdmin ? [{ href: '/audit-log', label: $t('auditLog.menu'), icon: 'list' }] : []),
  ]);

  // Menu ponsel menutup sendiri setelah pindah halaman.
  $effect(() => {
    void path;
    menuOpen = false;
  });
</script>

{#snippet brand()}
  <span class="flex items-center gap-2">
    <Mascot size={22} />
    <span class="text-[15px] font-semibold tracking-[-0.02em] text-fg">{$t('app.name')}</span>
  </span>
{/snippet}

<a href="#main" data-router-ignore class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2">
  Skip to content
</a>

<div class="flex min-h-screen flex-col md:h-screen md:flex-row md:gap-3 md:p-3">
  <!-- Ponsel: top bar 48px (menu + nama produk). Desktop: tidak ada top bar, sidebar memegang semuanya. -->
  <div class="flex h-14 shrink-0 items-center justify-between glass mx-3 mt-3 rounded-full px-4 md:hidden">
    {@render brand()}
    <button
      type="button"
      aria-expanded={menuOpen}
      aria-controls="app-menu"
      onclick={() => (menuOpen = !menuOpen)}
      class="outlined inline-flex h-9 items-center rounded-full bg-surface/80 px-4 text-sm font-medium shadow-xs"
    >
      {$t('nav.menu')}
    </button>
  </div>

  <aside
    id="app-menu"
    class="{menuOpen ? 'flex' : 'hidden'} glass mx-3 mt-2 shrink-0 flex-col gap-6 rounded-sheet px-3 py-3 md:mx-0 md:mt-0 md:flex md:w-[232px] md:py-4"
  >
    <div class="hidden px-2 pt-1 md:block">{@render brand()}</div>

    <nav aria-label={$t('nav.main')} class="flex flex-col gap-0.5">
      {#each links as l (l.href)}
        {@const active = isActive(l.href)}
        <a
          href={l.href}
          aria-current={active ? 'page' : undefined}
          class="flex h-10 items-center gap-2.5 rounded-full py-1 pr-3 pl-1.5 text-sm font-medium {active
            ? 'font-semibold text-brand-ink'
            : 'text-muted hover:bg-surface/60 hover:text-fg'}"
        >
          <span class="grid h-7 w-7 shrink-0 place-items-center rounded-xl {active ? 'bg-brand text-[#0f1216]' : ''}">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
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
          </span>
          {l.label}
        </a>
      {/each}
    </nav>

    <div class="mt-auto flex flex-col gap-2">
      {#if $session.identity}
        <a href="/profile" aria-label={$t('nav.profile')} class="flex items-center gap-2.5 rounded-full px-2 py-1.5 hover:bg-surface/60">
          <Avatar name={$session.identity.name} size={28} />
          <span class="min-w-0 truncate text-sm font-medium">{$session.identity.name}</span>
        </a>
      {/if}
      <div class="flex items-center gap-2 px-0.5">
        <div class="min-w-0 flex-1"><LocaleSwitch /></div>
        <ThemeSwitch />
      </div>
      {#if $session.identity}<div class="px-0.5 [&>button]:w-full"><Button size="sm" variant="secondary" loading={leaving} onclick={leave}>{$t('nav.logout')}</Button></div>{/if}
    </div>
  </aside>

  <main id="main" class="glass m-3 min-w-0 flex-1 rounded-sheet p-5 md:m-0 md:overflow-y-auto md:px-10 md:py-8">
    <div class="mx-auto w-full max-w-6xl">
      {@render children?.()}
    </div>
  </main>
</div>
