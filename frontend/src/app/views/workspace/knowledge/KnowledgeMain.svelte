<script lang="ts">
  import Button from '../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../shared/components/EmptyState.svelte';
  import { t } from '../../../../shared/stores/i18n';
  import type { KnowledgeUrlState } from '../../../workspace/knowledgeUrl';
  import type { DialogState } from './KnowledgeDialogs.svelte';
  import GitBranchIcon from '../GitBranchIcon.svelte';
  import FolderPane from './FolderPane.svelte';

  let {
    url,
    isEmpty,
    folderCount,
    go,
    opendialog,
  }: {
    url: KnowledgeUrlState;
    isEmpty: boolean;
    // jumlah file folder aktif dari pohon (null bila belum diketahui)
    folderCount: number | null;
    go: (s: Partial<KnowledgeUrlState>) => void;
    opendialog: (d: Exclude<DialogState, null>) => void;
  } = $props();
</script>

<section aria-label={$t('project.tab.knowledge')} class="min-w-0">
  {#if url.dir && !url.path}
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
        <div class="flex flex-wrap justify-center gap-2">
          <Button onclick={() => opendialog({ kind: 'upload', dir: '' })}>{$t('knowledge.actions.upload')}</Button>
          <Button variant="secondary" onclick={() => go({ mode: 'new' })}>{$t('knowledge.actions.newFile')}</Button>
          <Button variant="secondary" onclick={() => opendialog({ kind: 'gitSync' })}>
            <GitBranchIcon />
            {$t('gitSync.button')}
          </Button>
        </div>
      {/snippet}
    </EmptyState>
  {:else}
    <EmptyState title={$t('knowledge.pickFile')} />
  {/if}
</section>
