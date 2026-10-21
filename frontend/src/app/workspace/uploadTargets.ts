import type { UploadTarget } from '../../domain/project/knowledge/uploadfiles';

// Target upload tambahan untuk upload campuran. T06 menambah target api-spec
// (accepts .yaml/.yml/.json, memanggil service upload T06 lewat index.ts-nya) DI SINI.
// Selama kosong, .yaml/.yml/.json dilaporkan "tidak didukung".
export const extraUploadTargets: UploadTarget[] = [];
