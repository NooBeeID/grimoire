<script lang="ts">
  import ProjectContextView from './ProjectContextView.svelte';
  import type { ProjectContext } from '../types';

  let { projectId, capture }: { projectId: string; capture: (c: ProjectContext) => void } = $props();
</script>

<ProjectContextView {projectId}>
  {#snippet children(ctx)}
    {capture(ctx)}
    <p data-testid="child">child:{ctx.project.name}</p>
  {/snippet}
</ProjectContextView>
