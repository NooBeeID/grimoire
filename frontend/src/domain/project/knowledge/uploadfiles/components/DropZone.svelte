<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { fromDrop, fromInput } from '../services/collect';
  import type { LocalFile } from '../types';

  let {
    acceptExts = [],
    onpick,
  }: {
    // Ekstensi tambahan dari target upload lain (mis. .yaml/.yml/.json), selain .md/.markdown/.txt.
    acceptExts?: string[];
    onpick: (files: LocalFile[], truncated: boolean) => void;
  } = $props();

  let over = $state(false);
  let depth = 0;
  const accept = $derived(['.md', '.markdown', '.txt', ...acceptExts].join(','));
  const uid = $props.id();
  const hintId = `dz-${uid}`;

  function picked(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    if (!input.files?.length) return;
    onpick(fromInput(input.files), false);
    input.value = '';
  }

  function enter(e: DragEvent) {
    e.preventDefault();
    depth++;
    over = true;
  }
  function leave() {
    depth = Math.max(0, depth - 1);
    if (depth === 0) over = false;
  }
  function allow(e: DragEvent) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
  }
  async function drop(e: DragEvent) {
    e.preventDefault();
    depth = 0;
    over = false;
    if (!e.dataTransfer) return;
    const r = await fromDrop(e.dataTransfer);
    if (r.files.length) onpick(r.files, r.truncated);
  }

  const button =
    'inline-flex h-8 cursor-pointer items-center rounded-base border border-control bg-surface px-3.5 text-sm font-medium focus-within:ring-2 focus-within:ring-accent-ink hover:bg-raised';
</script>

<div
  role="group"
  aria-describedby={hintId}
  data-testid="dropzone"
  data-over={over}
  ondragenter={enter}
  ondragover={allow}
  ondragleave={leave}
  ondrop={drop}
  class="flex flex-col items-start gap-3 rounded-card border border-dashed p-5 {over ? 'border-accent-ink bg-active' : 'border-control'}"
>
  <p id={hintId} class="text-sm">{$t('knowledge.upload.drop')}</p>
  <div class="flex gap-2">
    <label class={button}>
      {$t('knowledge.upload.pickFolder')}
      <input type="file" class="sr-only" multiple data-testid="pick-folder" onchange={picked} {...{ webkitdirectory: true }} />
    </label>
    <label class={button}>
      {$t('knowledge.upload.pickFiles')}
      <input type="file" class="sr-only" multiple {accept} data-testid="pick-files" onchange={picked} />
    </label>
  </div>
</div>
