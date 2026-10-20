<script lang="ts">
  import { tick } from 'svelte';
  import { AuditEntryDetailView } from '../../domain/core/audit-log/getentry';
  import { ListEntriesView } from '../../domain/core/audit-log/listentries';
  import { current, navigate } from '../routes/router';

  // params diterima agar cocok dengan App.svelte; tidak dipakai.
  let { params: _params }: { params?: Record<string, string> } = $props();

  const query = $derived($current.location.query);
  const entryId = $derived(query.get('entry') || null);
  let lastTrigger: string | null = null;

  const urlFor = (q: URLSearchParams) => '/audit-log' + (q.size ? '?' + q.toString() : '');

  function onquerychange(q: URLSearchParams) {
    navigate(urlFor(q), { replace: true });
  }
  function onselect(id: string) {
    lastTrigger = id;
    const q = new URLSearchParams(query);
    q.set('entry', id);
    navigate(urlFor(q));
  }
  async function onclose() {
    const id = entryId ?? lastTrigger;
    const q = new URLSearchParams(query);
    q.delete('entry');
    navigate(urlFor(q), { replace: true });
    await tick();
    if (id) document.querySelector<HTMLElement>(`[data-entry-id="${CSS.escape(id)}"]`)?.focus();
  }

  $effect(() => {
    if (!entryId) return;
    const prev = document.body.style.overflow;
    const mq = window.matchMedia?.('(min-width: 1024px)');
    if (!mq?.matches) document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prev;
    };
  });
</script>

<div class="grid gap-4 {entryId ? 'lg:grid-cols-[1fr_28rem]' : ''}">
  <ListEntriesView {query} selectedId={entryId} {onquerychange} {onselect} />
  {#if entryId}
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Audit entry"
      class="fixed inset-0 z-40 overflow-auto bg-black/40 p-4 lg:static lg:z-auto lg:overflow-visible lg:bg-transparent lg:p-0"
    >
      <div class="lg:sticky lg:top-4"><AuditEntryDetailView id={entryId} {onclose} /></div>
    </div>
  {/if}
</div>
