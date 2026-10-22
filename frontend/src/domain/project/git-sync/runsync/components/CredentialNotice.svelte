<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { SETTINGS_PATH } from '../../base';

  let {
    state,
    host = '',
    settingsPath = SETTINGS_PATH,
  }: { state: 'missing' | 'unreadable' | 'disabled'; host?: string; settingsPath?: string } = $props();

  const parts = $derived($t('gitSync.credential.disabled', { key: '\u0001' }).split('\u0001'));
</script>

<div role="status" class="rounded-card bg-pill-warning-bg px-3 py-2 text-sm text-pill-warning-ink" data-testid="credential-notice" data-state={state}>
  {#if state === 'disabled'}
    <p>{parts[0]}<code class="font-mono text-[13px]">GIT_CREDENTIAL_KEY</code>{parts[1] ?? ''}</p>
  {:else}
    <p>{$t(`gitSync.credential.${state}`, { host })}</p>
    <a href={settingsPath} class="text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{$t('gitSync.credential.setup')}</a>
  {/if}
</div>
