<script lang="ts">
  import Button from '../../../../shared/components/Button.svelte';
  import Dialog from '../../../../shared/components/Dialog.svelte';
  import { t } from '../../../../shared/stores/i18n';
  import { toast } from '../../../../shared/stores/toast';
  import { copyText } from '../../../../shared/utils/clipboard';
  import { dirName, isSupported } from '../../../../domain/project/knowledge/base';
  import { resolveRelativePath } from '../../../../shared/utils/markdown/links';
  import type { TreeAction, TreeNodeRef } from '../../../../domain/project/knowledge/listtree';
  import { navigate, current } from '../../../routes/router';
  import { buildKnowledgeHref, parseKnowledgeQuery, type KnowledgeUrlState } from '../../../workspace/knowledgeUrl';
  import type { WorkspaceTabProps } from '../../../workspace/tabs';
  import KnowledgeMain from './KnowledgeMain.svelte';
  import KnowledgeSidebar from './KnowledgeSidebar.svelte';
  import KnowledgeDialogs, { type DialogState } from './KnowledgeDialogs.svelte';

  let { ctx, onprojectgone }: WorkspaceTabProps = $props();

  const projectId = $derived(ctx.project.id);
  const url = $derived(parseKnowledgeQuery($current.location.query));

  let treeRefreshKey = $state(0);
  let viewerReloadKey = $state(0);
  let dirty = $state(false);
  let isEmpty = $state(false);
  let dialog = $state<DialogState>(null);
  let pendingHref = $state<string | null>(null);
  let drawerOpen = $state(false);
  let folderCounts = new Map<string, number>();

  const go = (s: Partial<KnowledgeUrlState>, replace = false) => request(buildKnowledgeHref(projectId, s), replace);

  // Penjaga perubahan belum disimpan: tunda navigasi sampai dikonfirmasi.
  function request(href: string, replace = false) {
    if (dirty) pendingHref = href;
    else {
      drawerOpen = false;
      navigate(href, { replace });
    }
  }
  function confirmLeave() {
    const href = pendingHref;
    pendingHref = null;
    dirty = false;
    if (href) navigate(href);
  }

  const refresh = () => treeRefreshKey++;
  function stale() {
    refresh();
    void ctx.reload();
  }
  const refreshAndGo = (s: Partial<KnowledgeUrlState>) => {
    refresh();
    dirty = false;
    navigate(buildKnowledgeHref(projectId, s));
  };

  function resolveLink(href: string, fromDir: string): string | null {
    const p = resolveRelativePath(fromDir, href);
    return p && isSupported(p) ? buildKnowledgeHref(projectId, { path: p }) : null;
  }

  async function onaction(ref: TreeNodeRef, action: TreeAction) {
    if (ref.kind === 'folder') folderCounts.set(ref.path, ref.fileCount ?? 0);
    switch (action) {
      case 'open': go({ path: ref.path }); break;
      case 'copyPath': {
        const ok = await copyText(ref.path);
        toast.push({ type: ok ? 'success' : 'error', message: $t(ok ? 'knowledge.copy.done' : 'knowledge.copy.failed') });
        break;
      }
      case 'move': dialog = { kind: 'move', from: ref.path }; break;
      case 'deleteFile': dialog = { kind: 'deleteFile', path: ref.path }; break;
      case 'newFileHere': go({ mode: 'new', dir: ref.path }); break;
      case 'uploadHere': dialog = { kind: 'upload', dir: ref.path }; break;
      case 'deleteFolder': dialog = { kind: 'deleteFolder', path: ref.path, count: ref.fileCount ?? 0 }; break;
    }
  }

  const contextDir = $derived(url.dir ?? (url.path ? dirName(url.path) : ''));
  const selected = $derived(url.mode === 'new' ? null : (url.path ?? url.dir));
</script>

<div class="grid gap-6 lg:grid-cols-[20rem_minmax(0,1fr)]">
  <div class="lg:hidden">
    <Button variant="secondary" onclick={() => (drawerOpen = !drawerOpen)}>{$t('knowledge.panel.files')}</Button>
  </div>

  <KnowledgeSidebar
    {projectId}
    {url}
    {contextDir}
    {selected}
    refreshKey={treeRefreshKey}
    open={drawerOpen}
    onupload={() => (dialog = { kind: 'upload', dir: contextDir })}
    onnewfile={() => go({ mode: 'new', dir: contextDir })}
    onsearch={(q) => go({ ...url, q, mode: 'view' }, true)}
    onscopechange={(scope) => go({ q: url.q, scope, path: url.path, dir: url.dir }, true)}
    onopenfile={(path) => go({ path })}
    onopendir={(dir) => go({ dir })}
    {onaction}
    onprojectgone={() => void ctx.reload()}
    onempty={(e) => (isEmpty = e)}
  />

  <KnowledgeMain
    {projectId}
    {url}
    {isEmpty}
    {viewerReloadKey}
    folderCount={url.dir ? (folderCounts.get(url.dir) ?? null) : null}
    {resolveLink}
    {go}
    opendialog={(d) => (dialog = d)}
    ondirtychange={(d) => (dirty = d)}
    oncreated={(f) => refreshAndGo({ path: f.path })}
    oneditclosed={(f) => {
      dirty = false;
      viewerReloadKey++;
      navigate(buildKnowledgeHref(projectId, { path: f?.path ?? url.path }));
    }}
    onstale={stale}
  />
</div>

<KnowledgeDialogs
  {projectId}
  {dialog}
  onclose={() => (dialog = null)}
  onuploaded={refresh}
  onmoved={(path) => {
    dialog = null;
    refreshAndGo({ path });
  }}
  ondeletedfile={(path) => {
    dialog = null;
    refreshAndGo(dirName(path) ? { dir: dirName(path) } : {});
  }}
  ondeletedfolder={(path) => {
    dialog = null;
    refreshAndGo(dirName(path) ? { dir: dirName(path) } : {});
  }}
  onstale={stale}
  onprojectgone={() => void ctx.reload()}
/>

<Dialog open={pendingHref !== null} title={$t('knowledge.editor.discardConfirm')} onclose={() => (pendingHref = null)}>
  {#snippet footer()}
    <Button variant="secondary" onclick={() => (pendingHref = null)}>{$t('knowledge.conflict.keepEditing')}</Button>
    <Button variant="danger" onclick={confirmLeave}>{$t('knowledge.editor.discard')}</Button>
  {/snippet}
</Dialog>
