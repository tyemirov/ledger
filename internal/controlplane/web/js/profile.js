// @ts-check

const profiles = Object.freeze({
  "http://localhost:8000": Object.freeze({ apiOrigin: "http://localhost:8000" }),
  "https://ledger.mprlab.com": Object.freeze({ apiOrigin: "https://ledger-api.mprlab.com" }),
});

const profile = profiles[/** @type {keyof typeof profiles} */ (globalThis.location.origin)];
if (!profile) throw new Error("ledger_browser_profile_missing");

export const BROWSER_PROFILE = profile;
