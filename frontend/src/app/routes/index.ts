// Pola feature (tiket berikutnya):
//   frontend/src/domain/<domain>/<module>/<feature>/
//     services/<feature>.service.ts   pakai shared/services/http.ts + schema Zod; tanpa state
//     stores/<feature>.store.ts       svelte/store; tanpa fetch
//     components/*.svelte             presentasi; tidak memanggil service
//     views/<Feature>View.svelte      satu-satunya yang memanggil service + store
//     types.ts, index.ts              index.ts = API publik feature
// Daftarkan route dengan meng-import index.ts feature, mis.
//   import { LoginView } from '../../domain/core/auth/login';
//   { path: '/login', view: LoginView, meta: { layout: 'blank' } }
import HomeView from '../views/HomeView.svelte';
import NotFoundView from '../views/NotFoundView.svelte';
import type { RouteDef } from './router';

export const routes: RouteDef[] = [
  { path: '/', view: HomeView, meta: { layout: 'blank', titleKey: 'home.title' } },
];

export const notFoundRoute: RouteDef = {
  path: '*',
  view: NotFoundView,
  meta: { layout: 'blank', titleKey: 'notfound.title' },
};
