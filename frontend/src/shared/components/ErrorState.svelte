<script lang="ts">
  import { isApiError } from '../services/http';
  import { t } from '../stores/i18n';
  import Button from './Button.svelte';

  let { error, onretry }: { error: unknown; onretry?: () => void } = $props();

  const message = $derived(isApiError(error) ? error.message : $t('error.internal'));
  const traceId = $derived(isApiError(error) ? error.traceId : undefined);
</script>

<div role="alert" class="flex flex-col items-center gap-1.5 px-6 py-14 text-center">
  <p class="font-semibold text-danger">{message}</p>
  {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
  {#if onretry}<Button variant="secondary" size="sm" onclick={onretry}>{$t('common.retry')}</Button>{/if}
</div>
