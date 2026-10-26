<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let {
    myRole,
    isMember,
    accessLevel,
  }: { myRole: 'owner' | 'member' | null; isMember: boolean; accessLevel: 'owner' | 'member' | 'admin' } = $props();

  const label = $derived(
    myRole === 'owner'
      ? $t('project.role.owner')
      : myRole === 'member'
        ? $t('project.role.member')
        : accessLevel === 'admin' && !isMember
          ? $t('project.badge.notMember')
          : '-',
  );
  // Pemilik = pil kuning madu; lainnya pil netral. Peran tidak memakai warna status.
  const tone = $derived(myRole === 'owner' ? 'bg-accent text-on-accent' : 'bg-pill-neutral-bg text-pill-neutral-ink');
</script>

{#if label === '-'}
  <span class="text-muted">–</span>
{:else}
  <span class="inline-flex h-6 items-center rounded-full px-2.5 text-xs font-medium {tone}">{label}</span>
{/if}
