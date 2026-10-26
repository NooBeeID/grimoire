<script lang="ts">
  import IconButton from '../../../../../shared/components/IconButton.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { User } from '../types';

  let {
    user,
    isSelf = false,
    onedit,
    onstatus,
    onreset,
    onresend,
  }: {
    user: User;
    isSelf?: boolean;
    onedit: (u: User) => void;
    onstatus: (u: User) => void;
    onreset: (u: User) => void;
    onresend?: (u: User) => void;
  } = $props();

  // Akun undangan menunggu: Kirim ulang undangan menggantikan Reset password; akun nonaktif tidak punya keduanya.
  const inviteRow = $derived(user.invitation !== null && onresend !== undefined);
</script>

<div class="flex flex-wrap gap-1">
  <IconButton icon="edit" label={$t('users.action.edit')} onclick={() => onedit(user)} />
  <span title={isSelf ? $t('users.self.cannotDeactivate') : undefined}>
    <IconButton icon="toggle" tone={user.is_active ? 'danger' : 'default'} label={user.is_active ? $t('users.action.deactivate') : $t('users.action.activate')} disabled={isSelf} onclick={() => onstatus(user)} />
  </span>
  {#if inviteRow}
    {#if user.is_active}
      <IconButton icon="resend" label={$t('users.action.resendInvitation')} onclick={() => onresend?.(user)} />
    {/if}
  {:else}
    <span title={isSelf ? $t('users.self.useProfile') : undefined}>
      <IconButton icon="resetPassword" label={$t('users.action.resetPassword')} disabled={isSelf} onclick={() => onreset(user)} />
    </span>
  {/if}
</div>
{#if isSelf}
  <p class="sr-only">{$t('users.self.cannotDeactivate')}. {$t('users.self.useProfile')}</p>
{/if}
