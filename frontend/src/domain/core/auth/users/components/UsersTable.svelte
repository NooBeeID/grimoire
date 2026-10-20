<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { User } from '../types';
  import UserRowActions from './UserRowActions.svelte';

  let {
    users,
    selfId,
    loading = false,
    error = null,
    emptyText,
    onretry,
    onedit,
    onstatus,
    onreset,
  }: {
    users: User[];
    selfId?: string;
    loading?: boolean;
    error?: ApiError | null;
    emptyText?: string;
    onretry?: () => void;
    onedit: (u: User) => void;
    onstatus: (u: User) => void;
    onreset: (u: User) => void;
  } = $props();

  const columns = $derived([
    { key: 'name', header: $t('users.col.name') },
    { key: 'email', header: $t('users.col.email') },
    { key: 'role', header: $t('users.col.role') },
    { key: 'status', header: $t('users.col.status') },
    { key: 'last_login_at', header: $t('users.col.lastLogin') },
    { key: 'actions', header: $t('users.col.actions') },
  ]);
</script>

<Table {columns} rows={users} rowKey={(u: User) => u.id} {loading} {error} {emptyText} {onretry} caption={$t('users.title')}>
  {#snippet cell(u: User, col)}
    {#if col.key === 'name'}
      {u.name}{#if u.id === selfId}&nbsp;<span class="text-muted">{$t('users.you')}</span>{/if}
    {:else if col.key === 'email'}
      {u.email}
    {:else if col.key === 'role'}
      {$t(`users.role.${u.role}`)}
    {:else if col.key === 'status'}
      <span class="rounded-base border border-border px-2 py-0.5 text-xs">{u.is_active ? $t('users.status.active') : $t('users.status.inactive')}</span>
      {#if u.must_change_password}
        <span class="ml-1 rounded-base border border-border px-2 py-0.5 text-xs text-muted">{$t('users.badge.mustChange')}</span>
      {/if}
    {:else if col.key === 'last_login_at'}
      {u.last_login_at ? formatDateTime(u.last_login_at, $locale) : $t('users.neverLoggedIn')}
    {:else}
      <UserRowActions user={u} isSelf={u.id === selfId} {onedit} {onstatus} {onreset} />
    {/if}
  {/snippet}
</Table>
