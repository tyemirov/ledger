// @ts-check
'use strict';

/**
 * Config values for the demo UI. Copy and edit to match your stack; these are read by demo/ui.
 */
const resolvedOrigin =
  typeof window === 'object' &&
  window.location &&
  typeof window.location.origin === 'string' &&
  window.location.origin.trim() &&
  window.location.origin !== 'null'
    ? window.location.origin.trim().replace(/\/+$/, '')
    : 'https://localhost:4443';
window.DEMO_LEDGER_CONFIG = Object.freeze({
  tauthBaseUrl: resolvedOrigin,
  apiBaseUrl: resolvedOrigin,
  googleClientId: '991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com',
});
