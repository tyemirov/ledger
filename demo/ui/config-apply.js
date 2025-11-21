// @ts-check
'use strict';

(function applyDemoConfig() {
  if (typeof window !== 'object') {
    return;
  }

  const DEFAULTS = Object.freeze({
    tauthBaseUrl: 'http://localhost:8080',
    apiBaseUrl: 'http://localhost:9090',
    googleClientId: '991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com',
  });

  /**
   * @param {unknown} value
   * @returns {string}
   */
  function sanitizeUrl(value) {
    if (typeof value !== 'string') {
      return '';
    }
    const trimmed = value.trim();
    if (!trimmed) {
      return '';
    }
    return trimmed.replace(/\/+$/, '');
  }

  const injected = /** @type {any} */ (window.DEMO_LEDGER_CONFIG || {});
  const resolved = Object.freeze({
    tauthBaseUrl: sanitizeUrl(injected.tauthBaseUrl) || DEFAULTS.tauthBaseUrl,
    apiBaseUrl: sanitizeUrl(injected.apiBaseUrl) || DEFAULTS.apiBaseUrl,
    googleClientId:
      typeof injected.googleClientId === 'string' && injected.googleClientId.trim()
        ? injected.googleClientId.trim()
        : DEFAULTS.googleClientId,
  });

  window.DEMO_LEDGER_CONFIG = resolved;

  const headerElement = /** @type {HTMLElement | null} */ (document.getElementById('demo-header'));
  if (!headerElement) {
    return;
  }

  if (resolved.googleClientId) {
    headerElement.setAttribute('site-id', resolved.googleClientId);
  }
  if (resolved.tauthBaseUrl) {
    headerElement.setAttribute('base-url', resolved.tauthBaseUrl);
  }
  headerElement.setAttribute('login-path', '/auth/google');
  headerElement.setAttribute('logout-path', '/auth/logout');
  headerElement.setAttribute('nonce-path', '/auth/nonce');
})();
