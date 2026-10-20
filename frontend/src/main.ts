import './app.css';
import { mount } from 'svelte';
import App from './app/App.svelte';
import { notFoundRoute, routes } from './app/routes/index';
import { startRouter } from './app/routes/router';
import { isApiError } from './shared/services/http';
import { toast } from './shared/stores/toast';

// TODO(T01): bootstrap sesi lewat domain/core/auth/<feature>/index.ts (/api/auth/me dengan skipAuthRedirect).
startRouter(routes, notFoundRoute);

window.addEventListener('unhandledrejection', (e) => {
  if (isApiError(e.reason)) {
    toast.fromError(e.reason);
    e.preventDefault();
  }
});

mount(App, { target: document.getElementById('app')! });
