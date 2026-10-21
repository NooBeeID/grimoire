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
  const tone = $derived(myRole === 'owner' ? 'border-primary text-primary' : 'border-border text-muted');
</script>

<span class="inline-flex items-center rounded-base border px-2 py-0.5 text-xs font-medium {tone}">{label}</span>
