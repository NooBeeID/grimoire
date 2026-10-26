<script lang="ts">
  import IconButton from '../../../../../shared/components/IconButton.svelte';
  import Avatar from '../../../../../shared/components/Avatar.svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { AccessLevel, Member, MemberRole } from '../types';

  let {
    items,
    currentUserId,
    accessLevel,
    busyUserId = null,
    onrequestrole,
    onrequestremove,
    onrequestleave,
    now = new Date(),
  }: {
    items: Member[];
    currentUserId: string;
    accessLevel: AccessLevel;
    busyUserId?: string | null;
    onrequestrole: (m: Member, role: MemberRole) => void;
    onrequestremove: (m: Member) => void;
    onrequestleave: (m: Member) => void;
    now?: Date;
  } = $props();

  const canManage = $derived(accessLevel === 'owner' || accessLevel === 'admin');
  // Kolom status hanya tampil bila ada anggota yang tidak ditemukan/nonaktif/undangan menunggu; selain itu kosong di semua baris.
  const hasStatus = $derived(items.some((m) => !m.found || !m.is_active || m.invitation_pending));
  const columns = $derived([
    { key: 'name', header: $t('project.members.col.name') },
    { key: 'email', header: $t('project.members.col.email') },
    { key: 'role', header: $t('project.members.col.role') },
    ...(hasStatus ? [{ key: 'status', header: $t('project.members.col.status') }] : []),
    { key: 'added', header: $t('project.members.col.added') },
    { key: 'actions', header: $t('project.members.col.actions'), class: 'text-right' },
  ]);
  const displayName = (m: Member) => m.name ?? $t('project.members.unknownUser');
</script>

{#snippet cell(m: Member, col: { key: string })}
  {#if col.key === 'name'}
    <span class="flex items-center gap-2.5"><Avatar name={displayName(m)} size={28} /><span class="break-words">{displayName(m)}{#if m.user_id === currentUserId}<span class="ml-1 text-muted">{$t('project.members.you')}</span>{/if}</span></span>
  {:else if col.key === 'email'}
    <span class="break-all">{m.email ?? '-'}</span>
  {:else if col.key === 'role'}
    <span class="inline-flex h-6 items-center rounded-full px-2.5 text-xs font-medium {m.role === 'owner' ? 'bg-pill-neutral-bg text-pill-neutral-ink' : 'bg-raised text-muted'}">{m.role === 'owner' ? $t('project.role.owner') : $t('project.role.member')}</span>
  {:else if col.key === 'status'}
    {#if !m.found}<span class="text-danger">{$t('project.members.status.notFound')}</span>
    {:else if !m.is_active}<span class="text-muted">{$t('project.members.status.inactive')}</span>
    {:else if m.invitation_pending}
      {@const expired = m.invitation_expires_at !== null && new Date(m.invitation_expires_at) <= now}
      <span class="inline-flex h-5 flex-none items-center rounded-base px-2 text-xs font-medium whitespace-nowrap {expired ? 'bg-pill-warning-bg text-pill-warning-ink' : 'bg-pill-neutral-bg text-pill-neutral-ink'}">{expired ? $t('project.members.status.invitedExpired') : $t('project.members.status.invited')}</span>
    {/if}
  {:else if col.key === 'added'}
    <time datetime={m.added_at}>{formatDateTime(m.added_at, $locale)}</time>
    {#if m.added_by_label}<span class="block text-xs text-muted">{m.added_by_label}</span>{/if}
  {:else}
    {@const isSelf = m.user_id === currentUserId}
    {@const busy = busyUserId === m.user_id}
    {@const otherBusy = busyUserId !== null && !busy}
    <div class="flex flex-wrap justify-end gap-1">
      {#if isSelf}
        {#if canManage && m.role === 'owner'}
          <IconButton icon="demote" label={$t('project.members.makeMember')} loading={busy} disabled={otherBusy} onclick={() => onrequestrole(m, 'member')} />
        {/if}
        <IconButton icon="leave" tone="danger" label={$t('project.members.leave')} disabled={busy || otherBusy} onclick={() => onrequestleave(m)} />
      {:else if canManage}
        {#if m.role === 'member'}
          <IconButton icon="promote" label={$t('project.members.makeOwner')} loading={busy} disabled={otherBusy} onclick={() => onrequestrole(m, 'owner')} />
        {:else}
          <IconButton icon="demote" label={$t('project.members.makeMember')} loading={busy} disabled={otherBusy} onclick={() => onrequestrole(m, 'member')} />
        {/if}
        <IconButton icon="removeMember" tone="danger" label={$t('project.members.remove')} ariaLabel={$t('project.members.removeAria', { name: displayName(m) })} disabled={busy || otherBusy} onclick={() => onrequestremove(m)} />
      {/if}
    </div>
  {/if}
{/snippet}

<div class="overflow-x-auto">
  <Table {columns} rows={items} rowKey={(m) => m.user_id} {cell} caption={$t('project.members.title')} />
</div>
