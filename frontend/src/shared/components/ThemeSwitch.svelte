<script lang="ts">
  import { t } from '../stores/i18n';
  import { theme, toggleTheme } from '../stores/theme';

  // Tombol ikon: menampilkan tema yang akan dipilih (bulan saat terang, matahari saat gelap).
  const label = $derived($theme === 'dark' ? $t('theme.toLight') : $t('theme.toDark'));
</script>

<button
  type="button"
  aria-label={label}
  title={label}
  onclick={toggleTheme}
  class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-base border border-control bg-surface text-fg shadow-xs hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
>
  <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    {#if $theme === 'dark'}
      <circle cx="12" cy="12" r="4" /><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4" />
    {:else}
      <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" />
    {/if}
  </svg>
</button>
