// @ts-check
'use strict';

(function loadAuthHelper() {
  const config = window.DEMO_LEDGER_CONFIG || {};
  const baseUrl = typeof config.tauthBaseUrl === 'string' && config.tauthBaseUrl.trim()
    ? config.tauthBaseUrl.trim().replace(/\/+$/, '')
    : 'http://localhost:8080';
  const script = document.createElement('script');
  script.defer = true;
  script.crossOrigin = 'anonymous';
  script.src = `${baseUrl}/static/auth-client.js`;
  window.DEMO_LEDGER_AUTH_CLIENT_PROMISE = new Promise((resolve, reject) => {
    script.onload = () => resolve(true);
    script.onerror = () => reject(new Error('failed to load auth-client.js'));
  });
  document.head.appendChild(script);
})();
