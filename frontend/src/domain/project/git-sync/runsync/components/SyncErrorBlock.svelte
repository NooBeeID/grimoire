<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { syncErrorMessage } from '../errorMessage';
  import type { SyncError } from '../types';
  import CredentialNotice from './CredentialNotice.svelte';

  let { error, seconds, onretry }: { error: SyncError; seconds?: number; onretry?: () => void } = $props();

  const notice = $derived(
    error.code === 'CREDENTIAL_MISSING' ? 'missing' : error.code === 'CREDENTIAL_UNREADABLE' ? 'unreadable' : error.code === 'GIT_FEATURE_DISABLED' ? 'disabled' : null,
  );
  const retryable = $derived(['SYNC_IN_PROGRESS', 'GIT_UPSTREAM_ERROR'].includes(error.code));
</script>

<div role="alert" class="flex flex-col gap-2" data-testid="sync-error" data-code={error.code}>
  {#if notice}
    <CredentialNotice state={notice} host={error.host ?? ''} settingsPath={error.settingsPath} />
  {:else}
    <p class="text-sm text-danger">{syncErrorMessage($t, error, seconds)}</p>
    {#if error.code === 'GIT_TOKEN_REJECTED' && error.settingsPath}
      <a href={error.settingsPath} class="w-fit text-sm text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{$t('gitSync.credential.setup')}</a>
    {/if}
  {/if}
  {#if error.traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: error.traceId })}</p>{/if}
  {#if onretry && retryable}<div><Button variant="secondary" size="sm" onclick={onretry}>{$t('common.retry')}</Button></div>{/if}
</div>
