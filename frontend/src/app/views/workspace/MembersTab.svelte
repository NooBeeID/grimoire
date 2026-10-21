<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshEmailStatus } from '../../../domain/core/auth/getme';
  import { MembersView } from '../../../domain/project/management/members';
  import { mailStatus } from '../../../shared/stores/mailStatus';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, currentUserId, onprojectgone, onleft }: WorkspaceTabProps = $props();

  const canManage = $derived(ctx.project.access_level === 'owner' || ctx.project.access_level === 'admin');
  onMount(() => {
    if (canManage && $mailStatus === null) void refreshEmailStatus();
  });
</script>

<MembersView
  projectId={ctx.project.id}
  accessLevel={ctx.project.access_level}
  {currentUserId}
  emailEnabled={$mailStatus}
  {onleft}
  onaccesschanged={() => void ctx.reload()}
  {onprojectgone}
/>
