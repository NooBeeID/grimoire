<script lang="ts">
  import { locale, t } from '../stores/i18n';
  import { formatDateTime } from '../utils/datetime';
  import { absoluteUrl } from '../utils/url';
  import type { TemporaryCredentials } from '../validation/credentials';
  import Button from './Button.svelte';
  import CopyField from './CopyField.svelte';
  import Dialog from './Dialog.svelte';

  let {
    open = $bindable(false),
    credentials,
    emailStatus,
    expiresAt,
    onclose,
  }: {
    open?: boolean;
    credentials: TemporaryCredentials | null;
    emailStatus: 'failed' | 'not_configured';
    expiresAt: string;
    onclose: () => void;
  } = $props();

  const loginUrl = $derived(credentials ? absoluteUrl(credentials.login_url, window.location.origin) : '');
  const expires = $derived(formatDateTime(expiresAt, $locale));
  const message = $derived(
    credentials
      ? $t('invitation.credentials.messageBody', {
          url: loginUrl,
          email: credentials.email,
          password: credentials.temporary_password,
          date: expires,
        })
      : '',
  );

  let done = false;
  $effect(() => {
    if (open) done = false;
  });
  function finish() {
    if (done) return;
    done = true;
    open = false;
    onclose();
  }
</script>

<!-- Dialog tidak bisa ditutup lewat latar atau Escape; satu-satunya jalan keluar adalah tombol utama. -->
<Dialog bind:open title={$t('invitation.credentials.title')} dismissible={false} size="md" onclose={finish}>
  {#if credentials}
    <div class="flex flex-col gap-4">
      <p class="text-sm">{$t(`invitation.credentials.reason.${emailStatus}`, { email: credentials.email })}</p>
      <p role="alert" class="rounded-base border border-warning px-3 py-2 text-sm font-medium text-warning">
        {$t('invitation.credentials.onceWarning')}
      </p>
      <CopyField label={$t('invitation.credentials.email')} value={credentials.email} />
      <CopyField label={$t('invitation.credentials.password')} value={credentials.temporary_password} testid="temporary-password" />
      <CopyField label={$t('invitation.credentials.loginUrl')} value={loginUrl} />
      <p class="text-sm text-muted">{$t('invitation.credentials.expires', { date: expires })}</p>
      <CopyField label={$t('invitation.credentials.message')} value={message} multiline rows={5} />
      <div class="flex justify-end">
        <Button onclick={finish}>{$t('invitation.credentials.done')}</Button>
      </div>
    </div>
  {/if}
</Dialog>
