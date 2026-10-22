export type GitProvider = 'github' | 'gitlab';
export interface DeleteTarget {
  provider: GitProvider;
  host: string;
  last4: string;
}
