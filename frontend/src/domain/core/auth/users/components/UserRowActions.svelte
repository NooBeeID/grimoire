<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
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
  <Button size="sm" variant="ghost" onclick={() => onedit(user)}>{$t('users.action.edit')}</Button>
  <span title={isSelf ? $t('users.self.cannotDeactivate') : undefined}>
    <Button size="sm" variant="ghost" disabled={isSelf} onclick={() => onstatus(user)}>
      {user.is_active ? $t('users.action.deactivate') : $t('users.action.activate')}
    </Button>
  </span>
  {#if inviteRow}
    {#if user.is_active}
      <Button size="sm" variant="ghost" onclick={() => onresend?.(user)}>{$t('users.action.resendInvitation')}</Button>
    {/if}
  {:else}
    <span title={isSelf ? $t('users.self.useProfile') : undefined}>
      <Button size="sm" variant="ghost" disabled={isSelf} onclick={() => onreset(user)}>{$t('users.action.resetPassword')}</Button>
    </span>
  {/if}
</div>
{#if isSelf}
  <p class="sr-only">{$t('users.self.cannotDeactivate')}. {$t('users.self.useProfile')}</p>
{/if}
