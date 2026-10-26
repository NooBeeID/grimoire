<script lang="ts">
  import type { Snippet } from 'svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { SectionState } from '../stores/usage.store';

  let {
    title,
    description,
    state,
    children,
  }: {
    title: string;
    description?: string;
    state: SectionState<unknown>;
    children: Snippet;
  } = $props();
  const uid = $props.id();
</script>

<section class="min-w-0 rounded-card border border-border bg-surface p-4 sm:p-6" aria-labelledby="usage-{uid}">
  <h3 id="usage-{uid}" class="text-base font-semibold">{title}</h3>
  {#if description}<p class="mt-1 text-sm text-muted">{description}</p>{/if}
  <div class="mt-4">
    {#if state.status === 'needs_project'}
      <EmptyState title={$t('dashboard.usage.needsProject')} description={$t('dashboard.usage.needsProject.hint')} />
    {:else}
      {@render children()}
    {/if}
  </div>
</section>
