// @ts-check
// Controlled Google Identity boundary. The application and shared UI remain real.
window.google = { accounts: { id: {
    initialize(config) { this.config = config; },
    renderButton(host, options) {
        const button = document.createElement('button');
        button.textContent = 'G';
        button.setAttribute('aria-label', 'Sign in with Google');
        button.style.cssText = 'width:40px;height:40px;padding:0';
        button.addEventListener('click', () => {
            options.click_listener();
            this.config.callback({ credential: 'ledger-browser-google-credential', state: options.state });
        });
        host.replaceChildren(button);
    },
    prompt() { throw new Error('browser_test.google.button_required'); },
    disableAutoSelect() {},
    cancel() {},
} } };
