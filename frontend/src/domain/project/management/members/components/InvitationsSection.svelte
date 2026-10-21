<script lang="ts">
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { InvitationsState } from '../stores/invitationsList.store';
  import type { Invitation } from '../types';
  import InvitationsTable from './InvitationsTable.svelte';

  let {
    state,
    busyUserId = null,
    onretry,
    onrequestresend,
    onrequestrevoke,
  }: {
    state: InvitationsState;
    busyUserId?: string | null;
    onretry: () => void;
    onrequestresend: (i: Invitation) => void;
    onrequestrevoke: (i: Invitation) => void;
  } = $props();

  // Kosong dan muat pertama tidak dirender: tidak ada aksi untuk section kosong.
  const visible = $derived(state.status === 'error' || (state.status === 'ready' && state.items.length > 0));
</script>

{#if visible}
  <section class="flex flex-col gap-3" aria-label={$t('project.invitations.title')} data-testid="invitations-section">
    <header class="flex items-center gap-2">
      <h3 class="text-base font-semibold">
        {$t('project.invitations.title')}{#if state.status === 'ready'}<span class="ml-2 font-normal text-muted">{state.items.length}</span>{/if}
      </h3>
      {#if state.refreshing}<span class="text-sm text-muted" role="status">{$t('common.loading')}</span>{/if}
    </header>
    {#if state.status === 'error'}
      <ErrorState error={state.error} {onretry} />
    {:else}
      <InvitationsTable items={state.items} {busyUserId} {onrequestresend} {onrequestrevoke} />
    {/if}
  </section>
{/if}
