<script lang="ts">
  import Dialog from '../../../../shared/components/Dialog.svelte';
  import { t } from '../../../../shared/stores/i18n';
  import { baseName, dirName, type KnowledgeFile } from '../../../../domain/project/knowledge/base';
  import { NewFileView } from '../../../../domain/project/knowledge/createfile';
  import { FileViewerView } from '../../../../domain/project/knowledge/getfile';
  import { FileEditorView } from '../../../../domain/project/knowledge/updatefile';
  import type { KnowledgeUrlState } from '../../../workspace/knowledgeUrl';
  import type { DialogState } from './KnowledgeDialogs.svelte';

  let {
    projectId,
    url,
    dirty,
    viewerReloadKey,
    resolveLink,
    go,
    onclose,
    opendialog,
    ondirtychange,
    oncreated,
    oneditclosed,
    onstale,
  }: {
    projectId: string;
    url: KnowledgeUrlState;
    dirty: boolean;
    viewerReloadKey: number;
    resolveLink: (href: string, fromDir: string) => string | null;
    go: (s: Partial<KnowledgeUrlState>) => void;
    onclose: () => void;
    opendialog: (d: Exclude<DialogState, null>) => void;
    ondirtychange: (dirty: boolean) => void;
    oncreated: (file: KnowledgeFile) => void;
    oneditclosed: (file: KnowledgeFile | null) => void;
    onstale: () => void;
  } = $props();

  const open = $derived(url.mode === 'new' || url.path !== null);
  const title = $derived(url.mode === 'new' ? $t('knowledge.actions.newFile') : baseName(url.path ?? ''));
</script>

<!-- Saat ada perubahan belum disimpan, Esc/backdrop/silang tidak menutup langsung: minta konfirmasi lewat onclose. -->
<Dialog {open} {title} size="full" dismissible={!dirty} {onclose} onrequestclose={onclose}>
  {#if url.mode === 'new'}
    {#key url.dir}
      <NewFileView
        {projectId}
        dir={url.dir ?? ''}
        {oncreated}
        oncancel={onclose}
        {ondirtychange}
        onopenexisting={(path) => go({ path })}
        onprojectgone={onstale}
      />
    {/key}
  {:else if url.path && url.mode === 'edit'}
    {#key url.path}
      <FileEditorView
        {projectId}
        path={url.path}
        onclose={oneditclosed}
        {ondirtychange}
        onnotfound={onstale}
        onsaveasnew={() => go({ mode: 'new', dir: dirName(url.path ?? '') })}
      />
    {/key}
  {:else if url.path}
    <FileViewerView
      {projectId}
      path={url.path}
      reloadKey={viewerReloadKey}
      {resolveLink}
      onedit={(f) => go({ path: f.path, mode: 'edit' })}
      onmove={(f) => opendialog({ kind: 'move', from: f.path })}
      ondelete={(f) => opendialog({ kind: 'deleteFile', path: f.path })}
      ondir={(dir) => go(dir ? { dir } : {})}
      onnotfound={onstale}
    />
  {/if}
</Dialog>
