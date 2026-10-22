<script lang="ts">
  import { DeleteSpecDialog } from '../../../domain/project/api-spec/deletespec';
  import { DownloadSpecButton } from '../../../domain/project/api-spec/downloadspec';
  import { SpecDetailView } from '../../../domain/project/api-spec/getspec';
  import { ListSpecsView, createSpecListStore } from '../../../domain/project/api-spec/listspecs';
  import { RenameSpecDialog, type ApiSpec as RenamedSpec } from '../../../domain/project/api-spec/renamespec';
  import { UploadSpecsDialog } from '../../../domain/project/api-spec/uploadspecs';
  import { GitSyncDialog } from '../../../domain/project/git-sync/runsync';
  import Button from '../../../shared/components/Button.svelte';
  import { t } from '../../../shared/stores/i18n';
  import { navigate } from '../../routes/router';
  import GitBranchIcon from './GitBranchIcon.svelte';
  import type { WorkspaceTabProps } from '../../workspace/tabs';

  let { ctx, onprojectgone, item = null, query = new URLSearchParams() }: WorkspaceTabProps = $props();

  // Satu store per instance tab; dipakai bersama daftar dan dialog agar hasil aksi langsung tercermin.
  const list = createSpecListStore();
  let uploadOpen = $state(false);
  let syncOpen = $state(false);
  let renameOpen = $state(false);
  let deleteOpen = $state(false);

  const enc = encodeURIComponent;
  const base = $derived('/projects/' + enc(ctx.project.id) + '/api-specs');
  const hrefForOp = (k: string | null) => base + '/' + enc(item ?? '') + (k ? '?op=' + enc(k) : '');

  function toList() {
    list.invalidate();
    navigate(base, { replace: true });
  }
  function renamed(spec: RenamedSpec) {
    const old = item ?? '';
    list.rename(old, spec);
    navigate(base + '/' + enc(spec.name) + window.location.search, { replace: true });
  }
  function deleted(name: string) {
    list.remove(name);
    navigate(base, { replace: true });
  }
</script>

{#if item === null}
  <ListSpecsView projectId={ctx.project.id} store={list} onupload={() => (uploadOpen = true)} {onprojectgone}>
    {#snippet headeractions()}
      <Button variant="secondary" onclick={() => (syncOpen = true)}>
        <GitBranchIcon />
        {$t('gitSync.button')}
      </Button>
    {/snippet}
  </ListSpecsView>
  <GitSyncDialog
    open={syncOpen}
    projectId={ctx.project.id}
    kind="api_spec"
    onclose={() => (syncOpen = false)}
    onsynced={() => list.invalidate()}
    {onprojectgone}
  />
  <UploadSpecsDialog
    open={uploadOpen}
    projectId={ctx.project.id}
    existingNames={list.names()}
    onclose={() => (uploadOpen = false)}
    onuploaded={() => list.invalidate()}
    onstale={() => list.invalidate()}
    {onprojectgone}
  />
{:else}
  <div class="flex flex-col gap-4">
    <a href={base} class="w-fit text-sm text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{$t('apispec.detail.backToList')}</a>
    <SpecDetailView projectId={ctx.project.id} name={item} selectedOp={query.get('op')} {hrefForOp} backHref={base}>
      {#snippet actions(spec)}
        <DownloadSpecButton projectId={ctx.project.id} {spec} onspecgone={toList} />
        <Button variant="secondary" onclick={() => (renameOpen = true)}>{$t('apispec.rename.open')}</Button>
        <Button variant="danger" onclick={() => (deleteOpen = true)}>{$t('apispec.delete.open')}</Button>
      {/snippet}
    </SpecDetailView>
  </div>
  <RenameSpecDialog
    open={renameOpen}
    projectId={ctx.project.id}
    spec={{ name: item }}
    onclose={() => (renameOpen = false)}
    onrenamed={renamed}
    onspecgone={toList}
    {onprojectgone}
  />
  <DeleteSpecDialog open={deleteOpen} projectId={ctx.project.id} spec={{ name: item }} onclose={() => (deleteOpen = false)} ondeleted={deleted} />
{/if}
