// @ts-check

const http = require("http");
const fs = require("fs");
const path = require("path");
const { chromium } = require("playwright");

const DEMO_SITE_ID =
  "991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com";

const initialState = {
  coins: 20,
  entries: [],
  nextId: 1,
};

/**
 * Simple same-origin server for the demo UI and a stubbed ledger/demoapi.
 * - Serves static assets from demo/ui.
 * - Provides /api/* endpoints that mutate in-memory wallet state.
 * - Exposes /demo/config.js and /static/auth-client.js for header/auth flows.
 */
function startDemoServer() {
  const root = path.join(__dirname, "..", "ui");
  const state = { ...initialState };

  function respondJson(response, payload) {
    response.statusCode = 200;
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify(payload));
  }

  const server = http.createServer((request, response) => {
    const { method, url } = request;
    if (!url) {
      response.statusCode = 400;
      response.end("bad request");
      return;
    }

    if (url === "/demo/config.js") {
      const origin = request.headers.host
        ? `http://${request.headers.host}`
        : "http://localhost";
      response.setHeader("Content-Type", "application/javascript");
      response.end(
        `window.demoConfig=${JSON.stringify({
          baseUrl: origin,
          siteId: DEMO_SITE_ID,
          loginPath: "/auth/google",
          logoutPath: "/auth/logout",
          noncePath: "/auth/nonce",
          authClientUrl: `${origin}/static/auth-client.js`,
        })};`,
      );
      return;
    }

    if (url === "/static/auth-client.js") {
      response.setHeader("Content-Type", "application/javascript");
      response.end(`
        (() => {
          const profile = {
            user_id: "demo-user",
            user_email: "demo@example.com",
            display: "Demo User",
            avatar_url: "",
            roles: ["user"],
          };
          window.getCurrentUser = () => profile;
          window.logout = async () => {};
          window.apiFetch = (input, init) => fetch(input, init);
          window.initAuthClient = ({ onAuthenticated } = {}) => {
            onAuthenticated && onAuthenticated(profile);
          };
        })();
      `);
      return;
    }

    if (url === "/auth/nonce" && method === "POST") {
      return respondJson(response, { nonce: "demo-nonce" });
    }
    if (url === "/auth/google" && method === "POST") {
      return respondJson(response, {
        user_id: "demo-user",
        user_email: "demo@example.com",
        display: "Demo User",
        avatar_url: "",
        roles: ["user"],
      });
    }

    if (url === "/api/bootstrap" && method === "POST") {
      console.log("stub: bootstrap");
      state.coins = 20;
      state.entries = [];
      state.nextId = 1;
      return respondJson(response, { wallet: walletSnapshot(state) });
    }

    if (url === "/api/wallet" && method === "GET") {
      console.log("stub: wallet");
      return respondJson(response, { wallet: walletSnapshot(state) });
    }

      if (url === "/api/transactions" && method === "POST") {
      console.log("stub: spend");
      if (state.coins < 5) {
        return respondJson(response, {
          status: "insufficient_funds",
          wallet: walletSnapshot(state),
        });
      }
      state.coins -= 5;
      state.entries.unshift(entry(state, "spend", 5));
      return respondJson(response, {
        status: "success",
        wallet: walletSnapshot(state),
      });
    }

    if (url === "/api/purchases" && method === "POST") {
      console.log("stub: purchase");
      let body = "";
      request.on("data", (chunk) => {
        body += chunk.toString();
      });
      request.on("end", () => {
        let coins = 0;
        try {
          const parsed = JSON.parse(body || "{}");
          coins = typeof parsed.coins === "number" ? parsed.coins : 0;
        } catch {
          coins = 0;
        }
        if (coins <= 0) {
          response.statusCode = 400;
          response.end(`invalid coins: ${coins}`);
          return;
        }
        state.coins += coins;
        state.entries.unshift(entry(state, "purchase", coins));
        respondJson(response, { wallet: walletSnapshot(state) });
      });
      return;
    }

    // Static UI files (index.html rewritten with same-origin API base).
    const filePath = path.normalize(path.join(root, url.split("?")[0]));
    let finalPath = filePath;
    if (finalPath.endsWith("/")) {
      finalPath = path.join(finalPath, "index.html");
    }
    fs.readFile(finalPath, (err, data) => {
      if (err) {
        response.statusCode = 404;
        response.end("not found");
        return;
      }
      if (finalPath.endsWith("index.html")) {
        const origin = request.headers.host
          ? `http://${request.headers.host}`
          : "http://localhost";
        const content = data
          .toString("utf-8")
          .replace(/data-api-base-url="http:\/\/localhost:9090"/g, `data-api-base-url="${origin}"`)
          .replace(/data-api-base-url='http:\/\/localhost:9090'/g, `data-api-base-url="${origin}"`)
          .replace(/data-tauth-base-url="http:\/\/localhost:8080"/g, `data-tauth-base-url="${origin}"`)
          .replace(/base-url="http:\/\/localhost:8080"/g, `base-url="${origin}"`);
        response.setHeader("Content-Type", "text/html");
        response.end(content);
        return;
      }
      response.setHeader("Content-Type", contentType(finalPath));
      response.end(data);
    });
  });

  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, () => {
      const address = server.address();
      if (!address || typeof address !== "object") {
        reject(new Error("failed to bind demo server"));
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
function contentType(filePath) {
  if (filePath.endsWith(".html")) return "text/html";
  if (filePath.endsWith(".css")) return "text/css";
  if (filePath.endsWith(".js")) return "application/javascript";
  return "application/octet-stream";
}

/**
 * @param {{coins: number, entries: any[], nextId: number}} state
 * @param {"spend" | "purchase"} type
 * @param {number} coins
 */
function entry(state, type, coins) {
  return {
    entry_id: `E${state.nextId++}`,
    type,
    amount_cents: coins * 100,
    amount_coins: coins,
    reservation_id: "",
    idempotency_key: "",
    metadata: { source: "playwright", type },
    created_unix_utc: Math.floor(Date.now() / 1000),
  };
}

/**
 * @param {{coins: number, entries: any[]}} state
 */
function walletSnapshot(state) {
  return {
    balance: {
      total_cents: state.coins * 100,
      available_cents: state.coins * 100,
      total_coins: state.coins,
      available_coins: state.coins,
    },
    entries: [...state.entries],
  };
}

(async () => {
  const server = await startDemoServer();
  const baseUrl = `http://localhost:${server.port}`;

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  page.on("console", (message) => {
    // eslint-disable-next-line no-console
    console.log(`[console:${message.type()}] ${message.text()}`);
  });
  page.on("requestfailed", (request) => {
    // eslint-disable-next-line no-console
    console.log(`request failed: ${request.url()} -> ${request.failure()?.errorText}`);
  });
  page.on("pageerror", (error) => {
    // eslint-disable-next-line no-console
    console.log(`pageerror: ${error.message}`);
  });

  // Stub GIS script loaded by mpr-ui.
  await page.route("https://accounts.google.com/gsi/client", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: `
        window.google = { accounts: { id: { initialize() {}, renderButton() {}, prompt() {} } } };
        document.dispatchEvent(new Event("gsiLoaded"));
      `,
    }),
  );

  // Fallback when header still points at localhost:8080.
  await page.route("http://localhost:8080/static/auth-client.js", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: `
        window.getCurrentUser = () => ({ user_id: "demo-user" });
        window.logout = async () => {};
        window.apiFetch = (input, init) => fetch(input, init);
        window.initAuthClient = ({ onAuthenticated } = {}) => {
          onAuthenticated && onAuthenticated({ user_id: "demo-user" });
        };
      `,
    }),
  );

  await page.goto(`${baseUrl}/index.html`, { waitUntil: "networkidle" });

  // Wait for spend control to appear; fail on blank UI.
  const spendButton = page.locator("text=Spend 5 coins").first();
  await spendButton.waitFor({ timeout: 30000 });

  // Expect initial balances to show 20 coins.
  await page.waitForFunction(() => {
    const metrics = Array.from(document.querySelectorAll(".wallet-metric strong"));
    return metrics.some((node) => node.textContent && node.textContent.includes("20"));
  });

  // Spend 4 times to reach zero.
  for (let i = 0; i < 4; i++) {
    await spendButton.click();
  }

  await page.waitForFunction(() => {
    const metrics = Array.from(document.querySelectorAll(".wallet-metric strong"));
    return metrics.some((node) => node.textContent && node.textContent.includes("0 coins"));
  });

  // Fifth spend should prompt insufficient funds banner.
  await spendButton.click();
  await page.waitForFunction(() => {
    const banner = document.querySelector(".banner--error .banner__title");
    return banner && banner.textContent && banner.textContent.includes("Insufficient");
  });

  // Purchase 10 coins and verify.
  await page.getByRole("button", { name: "10 coins" }).click();
  await page.getByRole("button", { name: "Buy coins" }).click();
  await page.waitForFunction(() => {
    const metrics = Array.from(document.querySelectorAll(".wallet-metric strong"));
    return metrics.some((node) => node.textContent && node.textContent.includes("10 coins"));
  });

  // Spend twice to hit zero and see zero-balance notice.
  await spendButton.click();
  await spendButton.click();
  await page.waitForFunction(() => {
    const metrics = Array.from(document.querySelectorAll(".wallet-metric strong"));
    return metrics.some((node) => node.textContent && node.textContent.includes("0 coins"));
  });
  await page.waitForFunction(() => {
    const notice = document.querySelector(".banner--warning");
    return notice && notice.textContent && notice.textContent.includes("Balance is zero");
  });

  await browser.close();
  server.close();
})();
