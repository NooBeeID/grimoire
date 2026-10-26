<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { ActorType } from '../types';
  let { type }: { type: ActorType } = $props();
  const label = $derived($t(`auditLog.actorType.${type}`));
</script>

{#if type !== 'user' && type !== 'system'}
  <span role="img" aria-label={label} title={label} class="inline-grid h-6 w-6 shrink-0 place-items-center rounded-base bg-primary align-middle text-primary-fg"><svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="4" y="8" width="16" height="11" rx="3" /><path d="M12 4v4M9 13v1M15 13v1" /></svg></span>
{:else}
<svg
  role="img"
  aria-label={label}
  width="16"
  height="16"
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  class="inline-block shrink-0"
>
  <title>{label}</title>
  {#if type === 'user'}
    <circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" />
  {:else if type === 'system'}
    <circle cx="12" cy="12" r="3" /><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2" />
  {/if}
</svg>
{/if}
