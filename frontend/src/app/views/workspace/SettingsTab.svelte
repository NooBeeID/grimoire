<script lang="ts">
  import { canDeleteProject } from '../../../domain/project/management/getproject';
  import { DeleteProjectView } from '../../../domain/project/management/deleteproject';
  import { ProjectSettingsView } from '../../../domain/project/management/updateproject';
  import { navigate } from '../../routes/router';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, onprojectgone }: WorkspaceTabProps = $props();
</script>

<div class="flex flex-col gap-8">
  <ProjectSettingsView
    project={ctx.project}
    onupdated={(p) => ctx.replace(p)}
    onforbidden={() => void ctx.reload()}
    {onprojectgone}
  />
  <!-- Member tidak melihat zona bahaya (UX saja; otorisasi di backend, rule 12.7). -->
  {#if canDeleteProject(ctx.project.access_level)}
    <DeleteProjectView
      project={ctx.project}
      ondeleted={() => navigate('/', { replace: true })}
      onforbidden={() => void ctx.reload()}
      {onprojectgone}
    />
  {/if}
</div>
