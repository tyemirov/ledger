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
      response.setHeader("Content-Type", "application/javascript");
      response.end(
        `window.demoConfig=${JSON.stringify({
          baseUrl: "http://localhost",
          siteId: DEMO_SITE_ID,
          loginPath: "/auth/google",
          logoutPath: "/auth/logout",
          noncePath: "/auth/nonce",
          authClientUrl: "/static/auth-client.js",
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
      state.coins = 20;
      state.entries = [];
      state.nextId = 1;
      return respondJson(response, { wallet: walletSnapshot(state) });
    }

    if (url === "/api/wallet" && method === "GET") {
      return respondJson(response, { wallet: walletSnapshot(state) });
    }

    if (url === "/api/transactions" && method === "POST") {
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
        const originPlaceholders = [
          /data-api-base-url="http:\\/\\/localhost:9090"/g,
          /data-api-base-url='http:\\/\\/localhost:9090'/g,
        ];
        let content = data.toString("utf-8");
        originPlaceholders.forEach((re) => {
          content = content.replace(re, `data-api-base-url="http://localhost"`);
        });
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

  await page.goto(`${baseUrl}/index.html`, { waitUntil: "networkidle" });

  // Bootstrap should grant 20 coins automatically.
  await page.waitForFunction(() => {
    const metrics = document.querySelectorAll(".wallet-metric strong");
    return metrics.length === 2 && metrics[0].textContent === "20";
  });

  // Spend 4 times to reach zero.
  for (let i = 0; i < 4; i++) {
    await page.getByRole("button", { name: "Spend 5 coins" }).click();
  }

  await page.waitForFunction(() => {
    const metrics = document.querySelectorAll(".wallet-metric strong");
    return metrics.length === 2 && metrics[1].textContent === "0";
  });

  // Fifth spend should surface insufficient funds banner.
  await page.getByRole("button", { name: "Spend 5 coins" }).click();
  await page.waitForFunction(() => {
    const banner = document.querySelector(".banner--error .banner__title");
    return banner && banner.textContent && banner.textContent.includes("Insufficient");
  });

  // Purchase 10 coins and verify balance.
  await page.getByRole("button", { name: "10 coins" }).click();
  await page.getByRole("button", { name: "Buy coins" }).click();
  await page.waitForFunction(() => {
    const metrics = document.querySelectorAll(".wallet-metric strong");
    return metrics.length === 2 && metrics[1].textContent === "10";
  });

  // Spend twice to reach zero and show zero-balance notice.
  for (let i = 0; i < 2; i++) {
    await page.getByRole("button", { name: "Spend 5 coins" }).click();
  }
  await page.waitForFunction(() => {
    const metrics = document.querySelectorAll(".wallet-metric strong");
    return metrics.length === 2 && metrics[1].textContent === "0";
  });
  await page.waitForFunction(() => {
    const notice = document.querySelector(".banner--warning");
    return notice && notice.textContent && notice.textContent.includes("Balance is zero");
  });

  await browser.close();
  server.close();
})();
