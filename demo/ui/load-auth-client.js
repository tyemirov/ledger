// @ts-check
'use strict';

(function loadAuthHelper() {
  if (typeof window !== 'object' || typeof document !== 'object') {
    return;
  }

  const config = window.DEMO_LEDGER_CONFIG || {};
  const baseUrl = typeof config.tauthBaseUrl === 'string' && config.tauthBaseUrl.trim()
    ? config.tauthBaseUrl.trim().replace(/\/+$/, '')
    : 'http://localhost:8080';
  const scriptUrl = `${baseUrl}/tauth.js`;

  const escapeHtml = (value) =>
    String(value)
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#39;');

  document.write(
    `<script src="${escapeHtml(scriptUrl)}"></script>`,
  );
})();
