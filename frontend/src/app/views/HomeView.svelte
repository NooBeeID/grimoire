<script lang="ts">
  import { CreateProjectDialog } from '../../domain/project/management/createproject';
  import { ProjectListView } from '../../domain/project/management/listprojects';
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

<h1 data-testid="home-title" class="text-2xl font-bold">{$t('home.title')}</h1>
{#if $session.identity}<p class="mt-1">{$t('home.greeting', { name: $session.identity.name })}</p>{/if}
<p class="mt-2 text-muted">{$t('home.subtitle')}</p>

<div class="mt-6">
  <ProjectListView query={$current.location.query} {onquerychange} canCreate oncreate={() => (createOpen = true)} />
</div>

<CreateProjectDialog
  open={createOpen}
  isSuperAdmin={$session.identity?.role === 'super_admin'}
  onclose={() => (createOpen = false)}
  oncreated={(p) => {
    createOpen = false;
    navigate('/projects/' + encodeURIComponent(p.id) + '/' + defaultTabKey());
  }}
/>
