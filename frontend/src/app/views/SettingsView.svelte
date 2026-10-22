<script lang="ts">
  import { tick } from 'svelte';
  import { DeleteCredentialDialog, type DeleteTarget } from '../../domain/core/git-credential/deletecredential';
  import { GitTokensSection, gitCredentials, type SaveRequest } from '../../domain/core/git-credential/listcredentials';
  import { SaveTokenDialog, type SaveTarget } from '../../domain/core/git-credential/savecredential';
  import { t } from '../../shared/stores/i18n';
  import { current } from '../routes/router';

  let saveTarget = $state<SaveTarget | null>(null);
  let deleteTarget = $state<DeleteTarget | null>(null);
  let ready = $state(false);

  // Dialog menyimpan target terakhir agar konten tidak hilang saat animasi tutup.
  let lastSave = $state<SaveTarget>({ provider: null, host: null, gitlabHosts: [] });
  $effect(() => {
    if (saveTarget) lastSave = saveTarget;
  });

  function openSave(req: SaveRequest) {
    saveTarget = { provider: req.provider, host: req.host, gitlabHosts: req.gitlabHosts };
  }

  // #token-git: gulir dan fokus ke judul seksi setelah data siap, juga saat navigasi in-app mengubah hash.
  async function focusAnchor() {
    void $current;
    if (!ready || window.location.hash !== '#token-git') return;
    await tick();
    const h = document.getElementById('token-git-title');
    h?.scrollIntoView({ block: 'start' });
    h?.focus({ preventScroll: true });
  }
  $effect(() => {
    void focusAnchor();
  });
</script>

<div class="mx-auto flex max-w-2xl flex-col gap-6">
  <h1 class="text-[22px] leading-8 font-semibold">{$t('settings.title')}</h1>

  <section id="token-git" aria-labelledby="token-git-title" class="scroll-mt-6">
    <GitTokensSection
      onready={() => (ready = true)}
      onsave={(row, hosts) => openSave({ provider: row.provider, host: row.host, gitlabHosts: row.provider === 'gitlab' ? hosts : [] })}
      onadd={openSave}
      ondelete={(row) => row.saved && (deleteTarget = { provider: row.provider, host: row.host, last4: row.saved.last4 })}
    />
  </section>
</div>

<SaveTokenDialog open={saveTarget !== null} target={lastSave} onclose={() => (saveTarget = null)} onsaved={() => gitCredentials.invalidate()} />
<DeleteCredentialDialog open={deleteTarget !== null} target={deleteTarget} onclose={() => (deleteTarget = null)} ondeleted={() => gitCredentials.invalidate()} />
