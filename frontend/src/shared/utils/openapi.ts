export const SWAGGER2_VERSION = '2.0';
export const isSwagger2Version = (v: string | null | undefined): boolean => v === SWAGGER2_VERSION;

// "Swagger 2.0" untuk "2.0", selain itu "OpenAPI <v>". Satu-satunya tempat teks label versi dibentuk.
export function openApiLabel(v: string): string {
  return isSwagger2Version(v) ? 'Swagger 2.0' : `OpenAPI ${v}`;
}
