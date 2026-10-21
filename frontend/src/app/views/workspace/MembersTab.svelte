<script lang="ts">
  import { MembersView } from '../../../domain/project/management/members';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, currentUserId, onprojectgone, onleft }: WorkspaceTabProps = $props();
</script>

<MembersView
  projectId={ctx.project.id}
  accessLevel={ctx.project.access_level}
  {currentUserId}
  {onleft}
  onaccesschanged={() => void ctx.reload()}
  {onprojectgone}
/>
