<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { GitProvider } from '../types';

  let { provider }: { provider: GitProvider } = $props();
</script>

<span class="inline-flex items-center gap-2">
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="shrink-0 text-muted">
    {#if provider === 'github'}
      <circle cx="8" cy="8" r="6" />
      <circle cx="8" cy="8" r="2" />
      <path d="M8 2v4M8 10v4" />
    {:else}
      <circle cx="4" cy="3.5" r="1.5" />
      <circle cx="4" cy="12.5" r="1.5" />
      <circle cx="12" cy="6.5" r="1.5" />
      <path d="M4 5v6M4 9c0-2 2-2.5 6.5-2.5" />
    {/if}
  </svg>
  <span>{$t(`gitCredential.provider.${provider}`)}</span>
</span>
