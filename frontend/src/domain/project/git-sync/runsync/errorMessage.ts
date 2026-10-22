import { errorKey } from '../base';
import type { SyncError } from './types';

type Translate = (key: string, params?: Record<string, string | number>) => string;

// Pesan untuk satu SyncError. seconds = sisa hitung mundur RATE_LIMITED (bila ada).
export function syncErrorMessage(t: Translate, e: SyncError, seconds?: number): string {
  const pick = (key: string, params?: Record<string, string | number>, fallback = e.message): string => {
    const tr = t(key, params);
    return tr !== key ? tr : fallback || t('gitSync.errors.generic');
  };
  switch (e.code) {
    case 'GIT_NOT_FOUND':
      if (e.field === 'folder') return pick(errorKey('GIT_NOT_FOUND_folder'), { folder: e.folder ?? '' });
      if (e.field === 'branch') return pick(errorKey('GIT_NOT_FOUND_branch'));
      return pick(errorKey('GIT_NOT_FOUND'));
    case 'SYNC_LIMIT_EXCEEDED':
      return e.limit ? pick(errorKey('SYNC_LIMIT_EXCEEDED'), { files: e.limit.files, max_files: e.limit.max_files }) : pick(errorKey('SYNC_LIMIT_EXCEEDED'));
    case 'RATE_LIMITED':
      return seconds !== undefined && seconds > 0 ? pick(errorKey('RATE_LIMITED_RETRY'), { seconds }) : pick(errorKey('RATE_LIMITED'));
    case 'INVALID_REPO_URL':
    case 'INVALID_ARGUMENT':
      return e.message || pick(errorKey(e.code));
    case 'ABORTED':
      return '';
    default:
      return pick(errorKey(e.code));
  }
}
