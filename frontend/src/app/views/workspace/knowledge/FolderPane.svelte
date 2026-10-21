<script lang="ts">
  import Button from '../../../../shared/components/Button.svelte';
  import { t } from '../../../../shared/stores/i18n';

  let {
    dir,
    fileCount,
    onnewfile,
    onupload,
    ondelete,
  }: { dir: string; fileCount: number | null; onnewfile: () => void; onupload: () => void; ondelete: () => void } = $props();
</script>

<section class="flex flex-col gap-4">
  <header class="flex flex-wrap items-start justify-between gap-3 border-b border-rule pb-4">
    <div class="min-w-0">
      <h2 class="font-mono text-[15px] font-semibold break-all">{dir}/</h2>
      {#if fileCount !== null}<p class="mt-1 text-sm text-muted">{$t('knowledge.tree.fileCount', { count: fileCount })}</p>{/if}
    </div>
    <div class="flex gap-2">
      <Button onclick={onnewfile}>{$t('knowledge.actions.newFileHere')}</Button>
      <Button variant="secondary" onclick={onupload}>{$t('knowledge.actions.uploadHere')}</Button>
      <Button variant="secondary" onclick={ondelete}>{$t('knowledge.actions.deleteFolder')}</Button>
    </div>
  </header>
  <p class="text-sm text-muted">{$t('knowledge.pickFile')}</p>
</section>
