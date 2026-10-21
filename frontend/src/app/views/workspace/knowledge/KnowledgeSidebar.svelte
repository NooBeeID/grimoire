<script lang="ts">
  import Button from '../../../../shared/components/Button.svelte';
  import { t } from '../../../../shared/stores/i18n';
  import { KnowledgeTreeView, type TreeAction, type TreeNodeRef } from '../../../../domain/project/knowledge/listtree';
  import { KnowledgeSearchView, SearchBox } from '../../../../domain/project/knowledge/search';
  import type { KnowledgeUrlState } from '../../../workspace/knowledgeUrl';

  let {
    projectId,
    url,
    contextDir,
    selected,
    refreshKey,
    open,
    onupload,
    onnewfile,
    onsearch,
    onscopechange,
    onopenfile,
    onopendir,
    onaction,
    onprojectgone,
    onempty,
  }: {
    projectId: string;
    url: KnowledgeUrlState;
    contextDir: string;
    selected: string | null;
    refreshKey: number;
    // laci panel pada layar sempit
    open: boolean;
    onupload: () => void;
    onnewfile: () => void;
    onsearch: (q: string) => void;
    onscopechange: (scope: string) => void;
    onopenfile: (path: string) => void;
    onopendir: (path: string) => void;
    onaction: (ref: TreeNodeRef, action: TreeAction) => void;
    onprojectgone: () => void;
    onempty: (isEmpty: boolean) => void;
  } = $props();

  const searching = $derived(url.q.trim() !== '');
</script>

<aside class="flex flex-col gap-3 {open ? '' : 'max-lg:hidden'}" aria-label={$t('knowledge.panel.files')}>
  <div class="flex gap-2">
    <Button size="sm" onclick={onupload}>{$t('knowledge.actions.upload')}</Button>
    <Button size="sm" variant="secondary" onclick={onnewfile}>{$t('knowledge.actions.newFile')}</Button>
  </div>
  <SearchBox value={url.q} onchange={onsearch} />
  {#if searching}
    <KnowledgeSearchView
      {projectId}
      q={url.q}
      scope={url.scope}
      {contextDir}
      {onscopechange}
      onopen={onopenfile}
      {onprojectgone}
    />
  {:else}
    <KnowledgeTreeView
      {projectId}
      selectedPath={selected}
      selectedIsDir={url.mode !== 'new' && !url.path && !!url.dir}
      {refreshKey}
      {onopenfile}
      {onopendir}
      {onaction}
      {onprojectgone}
      {onempty}
    />
  {/if}
</aside>
