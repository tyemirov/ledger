// @ts-check

const fs = require("fs");
const http = require("http");
const path = require("path");
const url = require("url");
const { chromium } = require("playwright");

const DEMO_SITE_ID =
  "991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com";

const profile = {
  user_id: "user-123",
  user_email: "demo.user@example.com",
  display: "Demo User",
  avatar_url: "",
  roles: ["user"],
};

const walletState = {
  coins: 20,
  nextEntryId: 1,
  entries: [],
};

/**
 * Serves files from demo/ui for the browser under test.
 * @returns {Promise<{port: number, close: () => void}>}
 */
function startStaticServer() {
  const root = path.join(__dirname, "..", "ui");
  const server = http.createServer((request, response) => {
    const parsed = url.parse(request.url || "/");
    const resolved = path.join(
      root,
      path.normalize(parsed.pathname || "/").replace(/^(\.\.[/\\])+/, ""),
    );
    const resolvedWithIndex = resolved.endsWith(path.sep)
      ? path.join(resolved, "index.html")
      : resolved;
    fs.readFile(resolvedWithIndex, (error, data) => {
      if (error) {
        response.statusCode = 404;
        response.end("not found");
        return;
      }
      response.setHeader("Content-Type", contentTypeFor(resolvedWithIndex));
      response.end(data);
    });
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, () => {
      const address = server.address();
      if (!address || typeof address !== "object") {
        reject(new Error("failed to bind static server"));
        return;
      }
      resolve({
        port: address.port,
        close: () => server.close(),
      });
    });
  });
}

/**
 * @param {string} filePath
 * @returns {string}
 */
function contentTypeFor(filePath) {
  if (filePath.endsWith(".html")) return "text/html";
  if (filePath.endsWith(".css")) return "text/css";
  if (filePath.endsWith(".js")) return "application/javascript";
  if (filePath.endsWith(".json")) return "application/json";
  return "application/octet-stream";
}

/**
 * @param {number} port
 * @returns {Record<string, string>}
 */
function corsHeaders(port) {
  const origin = `http://localhost:${port}`;
  return {
    "Access-Control-Allow-Origin": origin,
    "Access-Control-Allow-Credentials": "true",
  };
}

/**
 * @returns {{balance: object, entries: object[]}}
 */
function snapshotWallet() {
  return {
    balance: {
      total_cents: walletState.coins * 100,
      available_cents: walletState.coins * 100,
      total_coins: walletState.coins,
      available_coins: walletState.coins,
    },
    entries: [...walletState.entries],
  };
}

/**
 * @param {() => Promise<void>} fn
 */
async function withBrowser(fn) {
  const browser = await chromium.launch({ headless: true });
  try {
    await fn(browser);
  } finally {
    await browser.close();
  }
}

async function main() {
  const server = await startStaticServer();
  const cors = corsHeaders(server.port);

  await withBrowser(async (browser) => {
    const context = await browser.newContext({
      viewport: { width: 1280, height: 720 },
    });
    const page = await context.newPage();
    page.on("console", (message) => {
      // eslint-disable-next-line no-console
      console.log(`[console:${message.type()}] ${message.text()}`);
    });

    // Stub TAuth config and helper endpoints.
    await page.route("http://localhost:8080/demo/config.js", (route) => {
      if (route.request().method() === "OPTIONS") {
        return route.fulfill({ status: 204, headers: cors });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/javascript",
        body: `window.demoConfig=${JSON.stringify({
          baseUrl: "http://localhost:8080",
          siteId: DEMO_SITE_ID,
          loginPath: "/auth/google",
          logoutPath: "/auth/logout",
          noncePath: "/auth/nonce",
          authClientUrl: "http://localhost:8080/static/auth-client.js",
        })};`,
        headers: cors,
      });
    });

    await page.route("http://localhost:8080/static/auth-client.js", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 300));
      return route.fulfill({
        status: 200,
        contentType: "application/javascript",
        body: `
          (() => {
            const profile = ${JSON.stringify(profile)};
            window.__authSignedOut = false;
            window.getCurrentUser = () => (window.__authSignedOut ? null : profile);
            window.logout = async () => { window.__authSignedOut = true; };
            window.apiFetch = (input, init = {}) => fetch(input, { ...init, credentials: "include" });
            window.initAuthClient = ({ onAuthenticated, onUnauthenticated } = {}) => {
              if (window.__authSignedOut) {
                onUnauthenticated && onUnauthenticated();
                return;
              }
              onAuthenticated && onAuthenticated(profile);
            };
          })();`,
      });
    });

    let issuedNonce = null;

    await page.route("http://localhost:8080/auth/nonce", (route) => {
      issuedNonce = "demo-nonce";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ nonce: issuedNonce }),
        headers: {
          ...cors,
          "Set-Cookie": [`tauth_nonce=${issuedNonce}; Path=/auth; HttpOnly`],
        },
      });
    });

    await page.route("http://localhost:8080/auth/google", (route) => {
      const cookies = route.request().headers()["cookie"] || "";
      if (!issuedNonce || !cookies.includes(`tauth_nonce=${issuedNonce}`)) {
        return route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({ error: "nonce_mismatch" }),
          headers: cors,
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(profile),
        headers: cors,
      });
    });

    await page.route("https://accounts.google.com/gsi/client", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/javascript",
        body: `
          window.google = {
            accounts: {
              id: {
                initialize: () => {},
                renderButton: () => {},
                prompt: () => {},
              },
            },
          };
          window.__gsi_loaded = true;
          document.dispatchEvent(new Event("gsiLoaded"));
        `,
        headers: cors,
      }),
    );

    // Stub wallet API endpoints.
    await page.route("http://localhost:9090/api/bootstrap", (route) => {
      if (route.request().method() === "OPTIONS") {
        return route.fulfill({ status: 204, headers: cors });
      }
      walletState.coins = 20;
      walletState.entries = [];
      walletState.nextEntryId = 1;
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ wallet: snapshotWallet() }),
        headers: cors,
      });
    });

    await page.route("http://localhost:9090/api/wallet", (route) => {
      if (route.request().method() === "OPTIONS") {
        return route.fulfill({ status: 204, headers: cors });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ wallet: snapshotWallet() }),
        headers: cors,
      });
    });

    await page.route("http://localhost:9090/api/purchases", async (route) => {
      if (route.request().method() === "OPTIONS") {
        return route.fulfill({ status: 204, headers: cors });
      }
      const request = route.request();
      const payload = request.postDataJSON() || {};
      const coins = typeof payload.coins === "number" ? payload.coins : 0;
      walletState.coins += coins;
      walletState.entries.unshift({
        entry_id: `E${walletState.nextEntryId++}`,
        type: "purchase",
        amount_cents: coins * 100,
        amount_coins: coins,
        reservation_id: "",
        idempotency_key: "",
        metadata: { source: "e2e" },
        metadata_text: "purchase",
        created_at_unix: Math.floor(Date.now() / 1000),
      });
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ wallet: snapshotWallet() }),
        headers: cors,
      });
    });

    await page.route("http://localhost:9090/api/transactions", (route) => {
      if (route.request().method() === "OPTIONS") {
        return route.fulfill({ status: 204, headers: cors });
      }
      if (walletState.coins < 5) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            status: "insufficient_funds",
            wallet: snapshotWallet(),
          }),
          headers: cors,
        });
        return;
      }
      walletState.coins -= 5;
      walletState.entries.unshift({
        entry_id: `E${walletState.nextEntryId++}`,
        type: "spend",
        amount_cents: 500,
        amount_coins: 5,
        reservation_id: "",
        idempotency_key: "",
        metadata: { source: "e2e" },
        metadata_text: "spend",
        created_at_unix: Math.floor(Date.now() / 1000),
      });
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "success",
          wallet: snapshotWallet(),
        }),
        headers: cors,
      });
    });

    await page.goto(`http://localhost:${server.port}/index.html`, {
      waitUntil: "networkidle",
    });

    await page.waitForFunction(() => {
      const metric = document.querySelector(".wallet-summary strong");
      return metric && metric.textContent && metric.textContent.includes("20");
    }, { timeout: 30000 });

    await page.reload({ waitUntil: "networkidle" });

    await page.waitForFunction(() => {
      const metric = document.querySelector(".wallet-summary strong");
      return metric && metric.textContent && metric.textContent.includes("20");
    }, { timeout: 30000 });
  });

  server.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
