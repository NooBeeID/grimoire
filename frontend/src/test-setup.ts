import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach, beforeEach } from 'vitest';
import { setLocale } from './shared/stores/i18n';

Object.defineProperty(navigator, 'language', { get: () => 'id-ID', configurable: true });
beforeEach(() => {
  setLocale('id');
  localStorage.clear();
});
afterEach(() => cleanup());

const proto = HTMLDialogElement.prototype as HTMLDialogElement;
if (typeof proto.showModal !== 'function') {
  proto.showModal = function (this: HTMLDialogElement) {
    this.setAttribute('open', '');
  };
}
if (typeof proto.close !== 'function') {
  proto.close = function (this: HTMLDialogElement) {
    this.removeAttribute('open');
    this.dispatchEvent(new Event('close'));
  };
}
