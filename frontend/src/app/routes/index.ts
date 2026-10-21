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
import { LoginView } from '../../domain/core/auth/login';
import ChangePasswordRouteView from '../views/ChangePasswordRouteView.svelte';
import HomeView from '../views/HomeView.svelte';
import AuditLogView from '../views/AuditLogView.svelte';
import NotFoundView from '../views/NotFoundView.svelte';
import ProfileView from '../views/ProfileView.svelte';
import ProjectWorkspaceView from '../views/ProjectWorkspaceView.svelte';
import UsersView from '../views/UsersView.svelte';
import type { RouteDef } from './router';

export const routes: RouteDef[] = [
  { path: '/', view: HomeView, meta: { auth: true, layout: 'app', titleKey: 'home.title' } },
  {
    path: '/projects/:projectID',
    view: ProjectWorkspaceView,
    meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' },
  },
  {
    path: '/projects/:projectID/:tab',
    view: ProjectWorkspaceView,
    meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' },
  },
  {
    path: '/projects/:projectID/:tab/:item',
    view: ProjectWorkspaceView,
    meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' },
  },
  { path: '/login', view: LoginView, meta: { guestOnly: true, layout: 'blank', titleKey: 'auth.login.title' } },
  {
    path: '/change-password',
    view: ChangePasswordRouteView,
    meta: { auth: true, passwordChangeRoute: true, layout: 'blank', titleKey: 'auth.changePassword.forcedTitle' },
  },
  { path: '/profile', view: ProfileView, meta: { auth: true, layout: 'app', titleKey: 'auth.profile.title' } },
  { path: '/admin/users', view: UsersView, meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'users.title' } },
  {
    path: '/audit-log',
    view: AuditLogView,
    meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'auditLog.title' },
  },
];

export const notFoundRoute: RouteDef = {
  path: '*',
  view: NotFoundView,
  meta: { layout: 'blank', titleKey: 'notfound.title' },
};
