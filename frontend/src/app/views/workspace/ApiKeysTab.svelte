<script lang="ts">
  import { CreateKeyDialog } from '../../../domain/project/api-key/createkey';
  import { ListKeysView, createKeyListStore, type ApiKey } from '../../../domain/project/api-key/listkeys';
  import { RevokeKeyDialog, type RevokeTarget } from '../../../domain/project/api-key/revokekey';
  import { session } from '../../../shared/stores/session';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, onprojectgone }: WorkspaceTabProps = $props();

  const store = createKeyListStore();
  let createOpen = $state(false);
  let revokeTarget = $state<RevokeTarget | null>(null);

  const isSuperAdmin = $derived($session.identity?.role === 'super_admin');
  const auditHref = (key: ApiKey) =>
    '/audit-log?' +
    new URLSearchParams([
      ['project_id', ctx.project.id],
      ['action', 'apikey.created'],
      ['action', 'apikey.revoked'],
      ['target', key.id],
    ]).toString();
</script>

<ListKeysView
  projectId={ctx.project.id}
  {store}
  oncreate={() => (createOpen = true)}
  onrevoke={(k) => (revokeTarget = { id: k.id, name: k.name, prefix: k.prefix })}
  {onprojectgone}
  auditHref={isSuperAdmin ? auditHref : undefined}
  showOwner={isSuperAdmin}
  {isSuperAdmin}
/>

<CreateKeyDialog
  open={createOpen}
  projectId={ctx.project.id}
  onclose={() => (createOpen = false)}
  oncreated={(k) => store.upsert(k)}
  onstale={() => store.invalidate()}
  {onprojectgone}
/>

<RevokeKeyDialog
  open={revokeTarget !== null}
  projectId={ctx.project.id}
  target={revokeTarget}
  onclose={() => (revokeTarget = null)}
  onrevoked={(k) => store.upsert(k)}
  onstale={() => store.invalidate()}
  {onprojectgone}
/>
