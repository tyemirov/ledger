// @ts-check

/**
 * Loads the TAuth auth-client script once.
 * @param {string} scriptUrl
 * @returns {Promise<void>}
 */
export async function ensureAuthClient(scriptUrl) {
  if (typeof window.initAuthClient === "function") {
    return;
  }
  await loadScript(scriptUrl);
}

/**
 * @param {string} scriptUrl
 * @returns {Promise<void>}
 */
function loadScript(scriptUrl) {
  return new Promise((resolve, reject) => {
    const existing = Array.from(document.scripts || []).find(
      (element) => element.src === scriptUrl
    );
    if (existing) {
      resolve();
      return;
    }
    const script = document.createElement("script");
    script.defer = true;
    script.crossOrigin = "anonymous";
    script.src = scriptUrl;
    script.onload = () => resolve();
    script.onerror = () =>
      reject(new Error(`failed to load auth-client.js from ${scriptUrl}`));
    document.head.append(script);
  });
}
