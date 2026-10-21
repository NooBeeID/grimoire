<script lang="ts">
  import Button from '../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../shared/components/EmptyState.svelte';
  import { t } from '../../../../shared/stores/i18n';
  import { dirName, type KnowledgeFile } from '../../../../domain/project/knowledge/base';
  import { NewFileView } from '../../../../domain/project/knowledge/createfile';
  import { FileViewerView } from '../../../../domain/project/knowledge/getfile';
  import { FileEditorView } from '../../../../domain/project/knowledge/updatefile';
  import type { KnowledgeUrlState } from '../../../workspace/knowledgeUrl';
  import type { DialogState } from './KnowledgeDialogs.svelte';
  import FolderPane from './FolderPane.svelte';

  let {
    projectId,
    url,
    isEmpty,
    viewerReloadKey,
    folderCount,
    resolveLink,
    go,
    opendialog,
    ondirtychange,
    oncreated,
    oneditclosed,
    onstale,
  }: {
    projectId: string;
    url: KnowledgeUrlState;
    isEmpty: boolean;
    viewerReloadKey: number;
    // jumlah file folder aktif dari pohon (null bila belum diketahui)
    folderCount: number | null;
    resolveLink: (href: string, fromDir: string) => string | null;
    go: (s: Partial<KnowledgeUrlState>) => void;
    opendialog: (d: Exclude<DialogState, null>) => void;
    ondirtychange: (dirty: boolean) => void;
    oncreated: (file: KnowledgeFile) => void;
    oneditclosed: (file: KnowledgeFile | null) => void;
    onstale: () => void;
  } = $props();
</script>

<section aria-label={$t('project.tab.knowledge')} class="min-w-0">
  {#if url.mode === 'new'}
    {#key url.dir}
      <NewFileView
        {projectId}
        dir={url.dir ?? ''}
        {oncreated}
        oncancel={() => go(url.dir ? { dir: url.dir } : {})}
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
  {:else if url.dir}
    <FolderPane
      dir={url.dir}
      fileCount={folderCount}
      onnewfile={() => go({ mode: 'new', dir: url.dir })}
      onupload={() => opendialog({ kind: 'upload', dir: url.dir ?? '' })}
      ondelete={() => opendialog({ kind: 'deleteFolder', path: url.dir ?? '', count: folderCount ?? 0 })}
    />
  {:else if isEmpty}
    <EmptyState title={$t('knowledge.empty.title')} description={$t('knowledge.empty.hint')}>
      {#snippet action()}
        <div class="flex gap-2">
          <Button onclick={() => opendialog({ kind: 'upload', dir: '' })}>{$t('knowledge.actions.upload')}</Button>
          <Button variant="secondary" onclick={() => go({ mode: 'new' })}>{$t('knowledge.actions.newFile')}</Button>
        </div>
      {/snippet}
    </EmptyState>
  {:else}
    <EmptyState title={$t('knowledge.pickFile')} />
  {/if}
</section>
