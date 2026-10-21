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
  // Pil netral (abu); pemilik sedikit lebih gelap. Tidak ada warna untuk peran.
  const tone = $derived(myRole === 'owner' ? 'bg-pill-neutral-bg text-pill-neutral-ink' : 'bg-raised text-muted');
</script>

{#if label === '-'}
  <span class="text-muted">–</span>
{:else}
  <span class="inline-flex h-5 items-center rounded-full px-2 text-xs font-medium {tone}">{label}</span>
{/if}
