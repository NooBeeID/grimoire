/** @type {import('dependency-cruiser').IConfiguration} */
// Penegak batas import frontend (rules.txt 12.3 sampai 12.5).
// Nama aturan stabil: dipakai scripts/test-arch.mjs. Tidak ada allowlist/pengecualian.
module.exports = {
  forbidden: [
    {
      name: 'shared-no-domain',
      severity: 'error',
      comment: 'rules.txt 12.5: shared/ DILARANG meng-import domain/',
      from: { path: '^src/shared/' },
      to: { path: '^src/domain/' },
    },
    {
      name: 'shared-no-app',
      severity: 'error',
      comment: 'rules.txt 12.5: shared/ adalah lapisan paling bawah; DILARANG meng-import app/',
      from: { path: '^src/shared/' },
      to: { path: '^src/app/' },
    },
    {
      name: 'domain-no-app',
      severity: 'error',
      comment: 'rules.txt 12.4: komposisi lintas module hanya di app/; domain/ DILARANG meng-import app/',
      from: { path: '^src/domain/' },
      to: { path: '^src/app/' },
    },
    {
      name: 'no-cross-feature-internals',
      severity: 'error',
      comment:
        'rules.txt 12.3/12.4: DILARANG import silang ke internals feature lain; hanya index.ts feature yang boleh diimpor dari luar feature',
      from: { path: '^src/domain/([^/]+)/([^/]+)/([^/]+)/' },
      to: {
        path: '^src/domain/',
        pathNot: ['^src/domain/$1/$2/$3/', '^src/domain/[^/]+/[^/]+/[^/]+/index\\.ts$'],
      },
    },
    {
      name: 'outside-domain-only-via-index',
      severity: 'error',
      comment: 'rules.txt 12.3: dari luar domain/ (app/, main.ts) hanya index.ts feature yang boleh diimpor',
      from: { path: '^src/(app/|main\\.ts$)' },
      to: {
        path: '^src/domain/',
        pathNot: '^src/domain/[^/]+/[^/]+/[^/]+/index\\.ts$',
      },
    },
    {
      name: 'no-circular',
      severity: 'error',
      comment: 'Dependensi sirkular dilarang (kebersihan graf import)',
      from: {},
      to: { circular: true },
    },
    {
      name: 'not-to-test',
      severity: 'error',
      comment: 'Kode produksi DILARANG mengimpor file test/spec',
      from: { pathNot: '\\.(test|spec)\\.ts$' },
      to: { path: '\\.(test|spec)\\.ts$' },
    },
  ],
  options: {
    doNotFollow: { path: 'node_modules' },
    includeOnly: '^src',
    tsPreCompilationDeps: true,
    tsConfig: { fileName: 'tsconfig.json' },
    exclude: { path: '\\.d\\.ts$' },
    enhancedResolveOptions: {
      exportsFields: ['exports'],
      conditionNames: ['import', 'default'],
    },
  },
};
