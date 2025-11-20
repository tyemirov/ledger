// @ts-check

(function applyConfig() {
  const config = window.__TAUTH_DEMO_CONFIG || {};
  const header =
    document.getElementById('demo-header') || document.querySelector('mpr-header');
  if (!header) {
    return;
  }
  if (config.googleClientId) {
    header.setAttribute('site-id', String(config.googleClientId));
  }
  header.removeAttribute('base-url');
  header.setAttribute('login-path', '/auth/google');
  header.setAttribute('logout-path', '/auth/logout');
  header.setAttribute('nonce-path', '/auth/nonce');
})();
