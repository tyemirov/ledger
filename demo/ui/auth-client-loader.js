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
  if (typeof window.initAuthClient !== "function") {
    throw new Error(`auth-client.js did not initialize from ${scriptUrl}`);
  }
}

/**
 * @param {string} scriptUrl
 * @returns {Promise<void>}
 */
function loadScript(scriptUrl) {
  return new Promise((resolve, reject) => {
    const existing = findScript(scriptUrl);
    let script;
    const onLoad = () => {
      cleanup(existing || script, onLoad, onError);
      if (existing) {
        existing.dataset.loaded = "true";
      } else if (script) {
        script.dataset.loaded = "true";
      }
      resolve();
    };
    const onError = () => {
      cleanup(existing || script, onLoad, onError);
      reject(new Error(`failed to load auth-client.js from ${scriptUrl}`));
    };

    if (existing) {
      if (isReady(existing)) {
        resolve();
        return;
      }
      existing.addEventListener("load", onLoad, { once: true });
      existing.addEventListener("error", onError, { once: true });
      return;
    }

    script = document.createElement("script");
    script.defer = true;
    script.crossOrigin = "anonymous";
    script.src = scriptUrl;
    script.addEventListener("load", onLoad, { once: true });
    script.addEventListener("error", onError, { once: true });
    document.head.append(script);
  });
}

/**
 * @param {string} scriptUrl
 * @returns {HTMLScriptElement | undefined}
 */
function findScript(scriptUrl) {
  return Array.from(document.scripts || []).find(
    (element) => element.src === scriptUrl
  );
}

/**
 * @param {HTMLScriptElement | undefined} target
 * @param {(event?: any) => void} onLoad
 * @param {(event?: any) => void} onError
 * @returns {void}
 */
function cleanup(target, onLoad, onError) {
  if (!target) return;
  target.removeEventListener("load", onLoad);
  target.removeEventListener("error", onError);
}

/**
 * @param {HTMLScriptElement} script
 * @returns {boolean}
 */
function isReady(script) {
  return (
    script.dataset.loaded === "true" ||
    script.getAttribute("data-loaded") === "true" ||
    script.readyState === "complete" ||
    script.readyState === "loaded"
  );
}
