export type Obj = Record<string, unknown>;
export const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);
export const asStr = (v: unknown): string | null => (typeof v === 'string' ? v : null);
export const asArr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
export const asObj = (v: unknown): Obj => (isObj(v) ? v : {});
