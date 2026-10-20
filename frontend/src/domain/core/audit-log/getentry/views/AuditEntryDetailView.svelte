<script lang="ts">
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import AuditEntryDetailPanel from '../components/AuditEntryDetailPanel.svelte';
  import { getAuditEntry } from '../services/getentry.service';
  import { createEntryDetailStore } from '../stores/entryDetail.store';

  let { id, onclose }: { id: string | null; onclose: () => void } = $props();

  const store = createEntryDetailStore();
  let ctrl: AbortController | null = null;
  let retryTick = $state(0);

  async function load(entryId: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.begin(entryId);
    try {
      const entry = await getAuditEntry(entryId, c.signal);
      if (c.signal.aborted) return;
      store.resolve(entryId, entry);
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      store.fail(entryId, isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
    }
  }

  $effect(() => {
    void retryTick;
    if (!id) {
      ctrl?.abort();
      store.reset();
      return;
    }
    void load(id);
    return () => ctrl?.abort();
  });
</script>

{#if id}
  <AuditEntryDetailPanel detail={$store} {onclose} onretry={() => retryTick++} />
{/if}
