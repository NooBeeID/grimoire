<script lang="ts">
  import EmptyState from '../../shared/components/EmptyState.svelte';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
  import { ProjectContextView } from '../../domain/project/management/getproject';
  import { navigate } from '../routes/router';
  import { defaultTabKey, findTab, workspaceTabs } from '../workspace/tabs';
  import DocumentTitle from './workspace/DocumentTitle.svelte';
  import WorkspaceTabsNav from './workspace/WorkspaceTabsNav.svelte';

  let { params }: { params: { projectID: string; tab?: string } } = $props();

  const projectId = $derived(params.projectID);
  const tab = $derived(params.tab ? findTab(params.tab) : null);
  const base = $derived('/projects/' + encodeURIComponent(projectId));
  const currentUserId = $derived($session.identity?.userId ?? '');

  $effect(() => {
    if (!params.tab) navigate(base + '/' + defaultTabKey(), { replace: true });
  });

  const gone = () => navigate('/', { replace: true });
</script>

{#if params.tab}
  <ProjectContextView {projectId}>
    {#snippet children(ctx)}
      <div class="flex flex-col gap-4">
        <WorkspaceTabsNav {projectId} tabs={workspaceTabs} active={tab?.key ?? ''} />
        {#if tab}
          {@const Tab = tab.view}
          <Tab {ctx} {currentUserId} onprojectgone={gone} onleft={gone} />
        {:else}
          <EmptyState title={$t('project.tab.notFound')}>
            {#snippet action()}<a href="{base}/{defaultTabKey()}" class="text-primary underline">{$t(workspaceTabs[0]!.labelKey)}</a>{/snippet}
          </EmptyState>
        {/if}
        <DocumentTitle name={ctx.project.name} />
      </div>
    {/snippet}
  </ProjectContextView>
{/if}

