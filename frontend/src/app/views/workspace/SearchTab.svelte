<script lang="ts">
  import { DeleteGroupDialog } from '../../../domain/project/search-synonym/deletegroup';
  import { ListGroupsView, createGroupListStore, type SynonymGroup } from '../../../domain/project/search-synonym/listgroups';
  import { SaveGroupDialog } from '../../../domain/project/search-synonym/savegroup';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, onprojectgone }: WorkspaceTabProps = $props();

  const store = createGroupListStore();
  let saveOpen = $state(false);
  let editing = $state<SynonymGroup | null>(null);
  let deleting = $state<SynonymGroup | null>(null);
</script>

<ListGroupsView
  projectId={ctx.project.id}
  {store}
  oncreate={() => {
    editing = null;
    saveOpen = true;
  }}
  onedit={(g) => {
    editing = g;
    saveOpen = true;
  }}
  ondelete={(g) => (deleting = g)}
  {onprojectgone}
/>

<SaveGroupDialog
  open={saveOpen}
  projectId={ctx.project.id}
  group={editing}
  onclose={() => (saveOpen = false)}
  onsaved={(g) => store.upsert(g)}
  onstale={() => store.invalidate()}
  {onprojectgone}
/>

<DeleteGroupDialog
  open={deleting !== null}
  projectId={ctx.project.id}
  group={deleting}
  onclose={() => (deleting = null)}
  ondeleted={(id) => store.remove(id)}
  onstale={() => store.invalidate()}
  {onprojectgone}
/>
