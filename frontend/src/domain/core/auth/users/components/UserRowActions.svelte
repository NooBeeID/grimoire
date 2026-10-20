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
  }: {
    user: User;
    isSelf?: boolean;
    onedit: (u: User) => void;
    onstatus: (u: User) => void;
    onreset: (u: User) => void;
  } = $props();
</script>

<div class="flex flex-wrap gap-1">
  <Button size="sm" variant="secondary" onclick={() => onedit(user)}>{$t('users.action.edit')}</Button>
  <span title={isSelf ? $t('users.self.cannotDeactivate') : undefined}>
    <Button size="sm" variant="secondary" disabled={isSelf} onclick={() => onstatus(user)}>
      {user.is_active ? $t('users.action.deactivate') : $t('users.action.activate')}
    </Button>
  </span>
  <span title={isSelf ? $t('users.self.useProfile') : undefined}>
    <Button size="sm" variant="secondary" disabled={isSelf} onclick={() => onreset(user)}>{$t('users.action.resetPassword')}</Button>
  </span>
</div>
{#if isSelf}
  <p class="sr-only">{$t('users.self.cannotDeactivate')}. {$t('users.self.useProfile')}</p>
{/if}
