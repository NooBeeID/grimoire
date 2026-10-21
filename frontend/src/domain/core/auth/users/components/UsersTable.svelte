<script lang="ts">
  import Avatar from '../../../../../shared/components/Avatar.svelte';
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
    { key: 'actions', header: $t('users.col.actions'), class: 'text-right' },
  ]);
</script>

<Table {columns} rows={users} rowKey={(u: User) => u.id} {loading} {error} {emptyText} {onretry} caption={$t('users.title')}>
  {#snippet cell(u: User, col)}
    {#if col.key === 'name'}
      <span class="flex items-center gap-2.5"><Avatar name={u.name} size={28} /><span>{u.name}{#if u.id === selfId}&nbsp;<span class="text-muted">{$t('users.you')}</span>{/if}</span></span>
    {:else if col.key === 'email'}
      {u.email}
    {:else if col.key === 'role'}
      {$t(`users.role.${u.role}`)}
    {:else if col.key === 'status'}
      <span class="inline-flex h-5 items-center rounded-full px-2 text-xs font-medium {u.is_active ? 'bg-pill-positive-bg text-pill-positive-ink' : 'bg-pill-neutral-bg text-pill-neutral-ink'}">{u.is_active ? $t('users.status.active') : $t('users.status.inactive')}</span>
      {#if u.must_change_password}
        <span class="ml-1 inline-flex h-5 items-center rounded-full bg-pill-warning-bg px-2 text-xs font-medium text-pill-warning-ink">{$t('users.badge.mustChange')}</span>
      {/if}
    {:else if col.key === 'last_login_at'}
      {#if u.last_login_at}{formatDateTime(u.last_login_at, $locale)}{:else}<span class="text-muted">{$t('users.neverLoggedIn')}</span>{/if}
    {:else}
      <UserRowActions user={u} isSelf={u.id === selfId} {onedit} {onstatus} {onreset} />
    {/if}
  {/snippet}
</Table>
