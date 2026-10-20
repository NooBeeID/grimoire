import './app.css';
import { mount } from 'svelte';
import App from './app/App.svelte';
import { boot, installAuthHandlers } from './app/boot';
import { notFoundRoute, routes } from './app/routes/index';
import { startRouter } from './app/routes/router';
import { isApiError } from './shared/services/http';
import { toast } from './shared/stores/toast';

startRouter(routes, notFoundRoute);
installAuthHandlers();
void boot();

window.addEventListener('unhandledrejection', (e) => {
  if (isApiError(e.reason)) {
    toast.fromError(e.reason);
    e.preventDefault();
  }
});

mount(App, { target: document.getElementById('app')! });
