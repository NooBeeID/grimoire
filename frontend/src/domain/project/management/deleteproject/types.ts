export interface DeletableProject {
  id: string;
  name: string;
  member_count: number;
  counts: { files: number | null; specs: number | null; keys: number | null };
}
