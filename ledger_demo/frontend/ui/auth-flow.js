// @ts-check
import { AUTH_BASE_URL } from './constants.js';

/**
 * @param {{ walletClient: ReturnType<typeof import('./wallet-api.js').createWalletClient>, onAuthenticated: (profile: any, options?: { bootstrap?: boolean }) => Promise<void>, onSignOut: () => void, onMissingClient: () => void }} options
 */
export function createAuthFlow(options) {
  if (!options || typeof options.walletClient !== 'object') {
    throw new Error('auth_flow.invalid_wallet_client');
  }
  const { walletClient, onAuthenticated, onSignOut, onMissingClient } = options;

  async function restoreSession() {
    try {
      const session = await walletClient.fetchSession();
      if (session && session.user_id) {
        await onAuthenticated(
          {
            display: session.display,
            user_email: session.email,
            avatar_url: session.avatar,
            roles: session.roles,
          },
          { bootstrap: false },
        );
      }
    } catch (error) {
      const message = typeof error?.message === 'string' ? error.message : '';
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
