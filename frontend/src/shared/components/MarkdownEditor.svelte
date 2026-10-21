<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import type { CmHandle } from './markdownEditor/codemirror';

  let {
    value = $bindable(''),
    label,
    readonly = false,
    disabled = false,
    rows = 22,
    onsave,
  }: {
    value?: string;
    label: string;
    readonly?: boolean;
    disabled?: boolean;
    rows?: number;
    onsave?: () => void;
  } = $props();

  const uid = $props.id();
  const areaId = `md-ed-${uid}`;
  let host = $state<HTMLDivElement | null>(null);
  let cm = $state<CmHandle | null>(null);
  let destroyed = false;

  // Textarea dipakai dulu (langsung bisa diketik, juga fallback bila CodeMirror gagal dimuat).
  onMount(() => {
    void import('./markdownEditor/codemirror')
      .then((m) => {
        if (destroyed || !host) return;
        cm = m.createEditor({
          parent: host,
          value,
          readonly: readonly || disabled,
          label,
          onchange: (v) => {
            if (v !== value) value = v;
          },
          onsave: () => onsave?.(),
        });
      })
      .catch(() => {
        cm = null;
      });
  });

  $effect(() => {
    cm?.setValue(value);
  });
  $effect(() => {
    cm?.setReadonly(readonly || disabled);
  });

  onDestroy(() => {
    destroyed = true;
    cm?.destroy();
  });

  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      e.stopPropagation();
      onsave?.();
    }
  }
</script>

<div class="flex flex-col gap-1">
  <label for={areaId} class="text-[13px] font-medium">{label}</label>
  <div bind:this={host} class="md-editor-host rounded-base border border-control bg-surface" class:hidden={!cm}></div>
  {#if !cm}
    <textarea
      id={areaId}
      {rows}
      {readonly}
      {disabled}
      bind:value
      {onkeydown}
      class="rounded-base border border-control bg-surface px-3 py-2 font-mono text-[13px] focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
    ></textarea>
  {/if}
</div>
