<script lang="ts">
  import ErrorState from '../../shared/components/ErrorState.svelte';
  import PageHeader from '../../shared/components/PageHeader.svelte';
  import Spinner from '../../shared/components/Spinner.svelte';
  import { t } from '../../shared/stores/i18n';
  import { current, navigate } from '../routes/router';

  // params diterima agar cocok dengan App.svelte; tidak dipakai.
  // load: celah tes agar kegagalan chunk bisa ditiru; produksi memakai import dinamis bawaan.
  const defaultLoad = () => import('../../domain/core/audit-log/summary');
  let {
    params: _params,
    load = defaultLoad,
  }: { params?: Record<string, string>; load?: typeof defaultLoad } = $props();

  const query = $derived($current.location.query);
  const urlFor = (q: URLSearchParams) => '/dashboard' + (q.size ? '?' + q.toString() : '');
  const onquerychange = (q: URLSearchParams) => navigate(urlFor(q), { replace: true });

  // Chart.js ikut chunk ini saja (AC13): jangan meng-import feature summary secara statis.
  const loadChunk = () => load();
  let chunk = $state(loadChunk());
  const reloadChunk = () => (chunk = loadChunk());
</script>

<PageHeader title={$t('dashboard.title')} description={$t('dashboard.subtitle')} testid="dashboard-title" />

{#await chunk}
  <div class="flex justify-center p-8"><Spinner /></div>
{:then m}
  <m.SummaryView {query} {onquerychange} />
{:catch e}
  <ErrorState error={e} onretry={reloadChunk} />
{/await}
