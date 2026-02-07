// @ts-check
'use strict';

/**
 * UI-level config: pulls from window.DEMO_LEDGER_CONFIG if injected (e.g., via demo/config.js),
 * otherwise falls back to local defaults for TAuth and demoapi.
 */
(function setupUiConfig() {
  const injected = /** @type {any} */ (window.DEMO_LEDGER_CONFIG || {});
  const fallbackOrigin =
    typeof window === 'object' &&
    window.location &&
    typeof window.location.origin === 'string' &&
    window.location.origin.trim() &&
    window.location.origin !== 'null'
      ? window.location.origin.trim().replace(/\/+$/, '')
      : 'https://localhost:8080';
  window.DEMO_LEDGER_CONFIG = Object.freeze({
    tauthBaseUrl: typeof injected.tauthBaseUrl === 'string' && injected.tauthBaseUrl.trim()
      ? injected.tauthBaseUrl.trim()
      : fallbackOrigin,
    apiBaseUrl: typeof injected.apiBaseUrl === 'string' && injected.apiBaseUrl.trim()
      ? injected.apiBaseUrl.trim()
      : fallbackOrigin,
    googleClientId: typeof injected.googleClientId === 'string' && injected.googleClientId.trim()
      ? injected.googleClientId.trim()
      : '991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com',
  });
})();
