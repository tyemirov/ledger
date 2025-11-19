// @ts-check

import { AUTH_BASE_URL } from './constants.js';

/**
 * @param {{ walletClient: ReturnType<typeof import('./wallet-api.js').createWalletClient>, onAuthenticated: (profile: any, options?: { bootstrap?: boolean }) => Promise<void>, onSignOut: () => void, onMissingClient?: () => void }} params
 */
export function createAuthFlow(params) {
  const { walletClient, onAuthenticated, onSignOut, onMissingClient } = params;
  if (!walletClient || typeof onAuthenticated !== 'function' || typeof onSignOut !== 'function') {
    throw new Error('auth_flow.invalid_parameters');
  }

  async function restoreSession() {
    try {
      const session = await walletClient.fetchSession();
      if (session && session.user_id) {
        await onAuthenticated(
          {
            display: session.display,
            user_email: session.email,
            avatar_url: session.avatar_url,
            roles: session.roles,
          },
          { bootstrap: false },
        );
      }
    } catch (error) {
      const message = String(error && error.message ? error.message : '');
      if (!message.includes('401')) {
        console.error('session restore failed', error);
      }
    }
  }

  function attachAuthClient() {
    if (typeof window.initAuthClient === 'function') {
      window.initAuthClient({
        baseUrl: AUTH_BASE_URL,
        onAuthenticated: (profile) => onAuthenticated(profile, { bootstrap: true }),
        onUnauthenticated: onSignOut,
      });
    } else if (typeof onMissingClient === 'function') {
      onMissingClient();
    }
  }

  return {
    restoreSession,
    attachAuthClient,
  };
}
