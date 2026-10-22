<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { syncErrorMessage } from '../errorMessage';
  import type { SyncError } from '../types';
  import CredentialNotice from './CredentialNotice.svelte';

  let {
    error,
    seconds,
    lost = false,
    onretry,
    onreloadstatus,
  }: { error: SyncError; seconds?: number; lost?: boolean; onretry?: () => void; onreloadstatus?: () => void } = $props();

  const notice = $derived(
    error.code === 'CREDENTIAL_MISSING' ? 'missing' : error.code === 'CREDENTIAL_UNREADABLE' ? 'unreadable' : error.code === 'GIT_FEATURE_DISABLED' ? 'disabled' : null,
  );
  const retryable = $derived(['SYNC_IN_PROGRESS', 'GIT_UPSTREAM_ERROR', 'RATE_LIMITED'].includes(error.code));
</script>

<div role="alert" class="flex flex-col gap-2" data-testid="sync-error" data-code={error.code}>
  {#if lost}
    <p class="flex items-start gap-2 text-sm text-warning" data-testid="sync-lost">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true" class="mt-0.5 shrink-0">
        <circle cx="8" cy="8" r="6" />
        <path d="M8 7.5v3.5M8 5.25v.01" />
      </svg>
      <span>{$t('gitSync.progress.lost')}</span>
    </p>
    {#if onreloadstatus}<div><Button variant="secondary" size="sm" onclick={onreloadstatus}>{$t('gitSync.progress.reloadStatus')}</Button></div>{/if}
  {:else if notice}
    <CredentialNotice state={notice} host={error.host ?? ''} settingsPath={error.settingsPath} />
  {:else}
    <p class="text-sm text-danger">{syncErrorMessage($t, error, seconds)}</p>
    {#if error.code === 'GIT_TOKEN_REJECTED' && error.settingsPath}
      <a href={error.settingsPath} class="w-fit text-sm text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{$t('gitSync.credential.setup')}</a>
    {/if}
  {/if}
  {#if error.traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: error.traceId })}</p>{/if}
  {#if !lost && onretry && retryable}<div><Button variant="secondary" size="sm" disabled={error.code === 'RATE_LIMITED' && (seconds ?? 0) > 0} onclick={onretry}>{$t('common.retry')}</Button></div>{/if}
</div>
