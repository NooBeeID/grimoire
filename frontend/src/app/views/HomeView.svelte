<script lang="ts">
  import { CreateProjectDialog } from '../../domain/project/management/createproject';
  import { ProjectListView } from '../../domain/project/management/listprojects';
  import PageHeader from '../../shared/components/PageHeader.svelte';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
  import { current, navigate } from '../routes/router';
  import { defaultTabKey } from '../workspace/tabs';

  let createOpen = $state(false);

  // Pencarian memakai replace (tidak memenuhi riwayat); pindah halaman memakai push agar Back kembali.
  function onquerychange(next: URLSearchParams) {
    const prev = $current.location.query;
    const searchChanged = (prev.get('q') ?? '') !== (next.get('q') ?? '');
    navigate('/' + (next.size ? '?' + next.toString() : ''), { replace: searchChanged });
  }
</script>

<PageHeader title={$t('home.title')} testid="home-title" />

<ProjectListView query={$current.location.query} {onquerychange} canCreate oncreate={() => (createOpen = true)} />

<CreateProjectDialog
  open={createOpen}
  isSuperAdmin={$session.identity?.role === 'super_admin'}
  onclose={() => (createOpen = false)}
  oncreated={(p) => {
    createOpen = false;
    navigate('/projects/' + encodeURIComponent(p.id) + '/' + defaultTabKey());
  }}
/>
