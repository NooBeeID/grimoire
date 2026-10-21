import { del } from '../../../../../shared/services/http';

// Persis sama (tanpa trim, peka huruf besar-kecil), seperti base.ConfirmDeletion di backend.
export function confirmMatches(typed: string, name: string): boolean {
  return typed === name;
}

export async function deleteProject(id: string, confirmName: string): Promise<void> {
  await del('/api/projects/' + encodeURIComponent(id), { body: { confirm_name: confirmName } });
}
