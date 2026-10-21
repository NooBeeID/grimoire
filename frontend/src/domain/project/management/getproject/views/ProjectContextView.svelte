<script lang="ts">
  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import ProjectHeader from '../components/ProjectHeader.svelte';
  import { getProject } from '../services/getproject.service';
  import { createProjectDetailStore } from '../stores/projectDetail.store';
  import type { ProjectContext, ProjectDetail } from '../types';

  let {
    projectId,
    children,
    onnotfound,
  }: { projectId: string; children: Snippet<[ProjectContext]>; onnotfound?: () => void } = $props();

  const store = createProjectDetailStore();
  let ctrl: AbortController | null = null;

  const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));

  async function load(id: string): Promise<void> {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.begin(id);
    try {
      const p = await getProject(id, c.signal);
      if (c.signal.aborted) return;
      store.resolve(id, p);
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      const err = toApi(e);
      store.fail(id, err);
      if (err.status === 404) onnotfound?.();
    }
  }

  $effect(() => {
    const id = projectId;
    void untrack(() => load(id));
    return () => ctrl?.abort();
  });

  const ctx = (p: ProjectDetail): ProjectContext => ({
    project: p,
    reload: () => load(projectId),
    replace: (next) => store.replace(next),
  });

  const shown = $derived(
    $store.status === 'ready' ? $store.project : $store.status === 'loading' ? $store.previous : null,
  );
</script>

{#if shown}
  <div class="flex flex-col gap-4" aria-busy={$store.status === 'loading'}>
    <ProjectHeader project={shown} />
    {@render children(ctx(shown))}
  </div>
{:else if $store.status === 'notFound'}
  <EmptyState title={$t('project.detail.notFound')} description={$t('project.detail.notFoundHint')}>
    {#snippet action()}<a href="/" class="text-primary underline">{$t('project.detail.backHome')}</a>{/snippet}
  </EmptyState>
{:else if $store.status === 'error'}
  <ErrorState error={$store.error} onretry={() => load(projectId)} />
{:else}
  <div class="flex flex-col gap-3" aria-busy="true">
    <div class="h-8 w-1/3 rounded-base bg-black/10"></div>
    <div class="p-4"><Spinner /></div>
  </div>
{/if}
