// @ts-check

import { createWalletClient } from './wallet-api.js';
import { API_BASE_URL, TRANSACTION_COINS, PURCHASE_OPTIONS, STATUS_MESSAGES } from './constants.js';
import { createAuthFlow } from './auth-flow.js';

const walletClient = createWalletClient({ baseUrl: API_BASE_URL });
const DEFAULT_TRANSACTION_STATUS = STATUS_MESSAGES.idle;

function registerWalletPage() {
  if (window.Alpine && typeof window.Alpine.data === 'function') {
    window.Alpine.data('walletPage', walletPage);
  } else {
    document.addEventListener(
      'alpine:init',
      () => {
        window.Alpine.data('walletPage', walletPage);
      },
      { once: true },
    );
  }
}

function walletPage() {
  return {
    profile: null,
    wallet: null,
    busy: false,
    transactionStatus: DEFAULT_TRANSACTION_STATUS,
    transactionStatusKind: 'info',
    bannerMessage: '',
    bannerKind: 'success',
    bannerVisible: false,
    bannerTimeoutId: null,
    purchaseOptions: PURCHASE_OPTIONS.slice(),
    selectedPurchase: PURCHASE_OPTIONS[0],
    init() {
      document.addEventListener('mpr-ui:auth:unauthenticated', (event) => {
        event.preventDefault();
        this.handleSignOut();
      });
      document.addEventListener('mpr-ui:auth:authenticated', (event) => {
        const profile = event?.detail?.profile;
        if (!profile) {
          return;
        }
        this.handleAuthenticated(
          {
            display: profile.display,
            user_email: profile.user_email,
            avatar_url: profile.avatar_url,
            roles: profile.roles,
          },
          { bootstrap: true },
        );
      });
      this.authFlow = createAuthFlow({
        walletClient,
        onAuthenticated: (profile, options) => this.handleAuthenticated(profile, options),
        onSignOut: () => this.handleSignOut(),
        onMissingClient: () => {
          console.warn('auth-client not loaded');
          this.showBanner(STATUS_MESSAGES.authClientMissing, 'error');
        },
      });
      void this.authFlow.restoreSession();
      this.authFlow.attachAuthClient();
    },
    get authState() {
      return this.isAuthenticated ? 'ready' : 'signed-out';
    },
    get isAuthenticated() {
      return Boolean(this.profile);
    },
    get availableCoins() {
      return this.wallet?.balance.available_coins ?? 0;
    },
    get totalCoins() {
      return this.wallet?.balance.total_coins ?? 0;
    },
    get availableCents() {
      return this.wallet?.balance.available_cents ?? 0;
    },
    get totalCents() {
      return this.wallet?.balance.total_cents ?? 0;
    },
    get walletEntries() {
      return this.wallet?.entries ?? [];
    },
    formatEntryDate(entry) {
      return new Date(entry.created_unix_utc * 1000).toLocaleString();
    },
    formatCoins(value) {
      const prefix = value > 0 ? '+' : '';
      return `${prefix}${value} coins`;
    },
    async handleAuthenticated(profile, options = { bootstrap: true }) {
      this.profile = {
        display: profile?.display || profile?.user_email || 'User',
        user_email: profile?.user_email || '',
        avatar_url: profile?.avatar_url || '',
        roles: Array.isArray(profile?.roles) ? profile.roles : [],
      };
      this.showBanner(`Signed in as ${profile?.display || 'user'}`, 'success');
      try {
        if (options.bootstrap !== false) {
          await walletClient.bootstrap({ source: 'ui' });
        }
        await this.refreshWallet();
      } catch (error) {
        this.showBanner(STATUS_MESSAGES.bootstrapError, 'error');
        console.error(error);
      }
    },
    handleSignOut() {
      this.profile = null;
      this.wallet = null;
      this.updateTransactionStatus(DEFAULT_TRANSACTION_STATUS, 'info');
      this.showBanner(STATUS_MESSAGES.signedOut, 'success');
    },
    async refreshWallet() {
      try {
        const response = await walletClient.getWallet();
        this.wallet = response.wallet;
      } catch (error) {
        console.error(error);
        this.showBanner(STATUS_MESSAGES.loadWalletError, 'error');
      }
    },
    async handleTransactionClick() {
      if (this.busy) {
        return;
      }
      this.busy = true;
      this.updateTransactionStatus(STATUS_MESSAGES.processing, 'info');
      try {
        const response = await walletClient.spend({ source: 'ui', coins: TRANSACTION_COINS });
        this.wallet = response.wallet;
        if (response.status === 'insufficient_funds') {
          this.updateTransactionStatus(STATUS_MESSAGES.spendInsufficient, 'error');
        } else {
          this.updateTransactionStatus(STATUS_MESSAGES.spendSuccess, 'success');
        }
        this.checkZeroBalance();
      } catch (error) {
        console.error(error);
        this.updateTransactionStatus(STATUS_MESSAGES.spendError, 'error');
      } finally {
        this.busy = false;
      }
    },
    async handlePurchaseSubmit() {
      if (this.busy) {
        return;
      }
      const selected = Number(this.selectedPurchase);
      if (!PURCHASE_OPTIONS.includes(selected)) {
        this.updateTransactionStatus(STATUS_MESSAGES.selectValidAmount, 'error');
        return;
      }
      this.busy = true;
      try {
        const response = await walletClient.purchase(selected, { source: 'ui', coins: selected });
        this.wallet = response.wallet;
        this.updateTransactionStatus(
          `${STATUS_MESSAGES.purchaseSuccessPrefix} ${selected} coins.`,
          'success',
        );
      } catch (error) {
        console.error(error);
        this.updateTransactionStatus(STATUS_MESSAGES.purchaseError, 'error');
      } finally {
        this.busy = false;
      }
    },
    updateTransactionStatus(text, kind) {
      this.transactionStatus = text;
      this.transactionStatusKind = kind;
    },
    showBanner(message, kind) {
      this.bannerMessage = message;
      this.bannerKind = kind;
      this.bannerVisible = true;
      window.clearTimeout(Number(this.bannerTimeoutId));
      this.bannerTimeoutId = window.setTimeout(() => {
        this.bannerVisible = false;
      }, 4000);
    },
    checkZeroBalance() {
      if (this.wallet && this.wallet.balance.available_coins === 0) {
        this.showBanner(STATUS_MESSAGES.zeroBalance, 'error');
      }
    },
  };
}

window.walletPage = walletPage;
registerWalletPage();
