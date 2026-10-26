import { ApiError } from '../../../../../shared/services/http';

export function sectionError(e: ApiError | 'unavailable' | null, message: string): ApiError | null {
  if (e === 'unavailable') return new ApiError({ status: 200, code: 'UNAVAILABLE', message });
  return e;
}
