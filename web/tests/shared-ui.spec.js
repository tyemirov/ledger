// @ts-check
import { test, expect } from "@playwright/test";
import { spawn, execFileSync } from "node:child_process";
import { createHmac } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { once } from "node:events";
import os from "node:os";
import path from "node:path";
import { installSharedUI } from "../shared-ui-candidate.js";

test.use({ actionTimeout: 5000 });

const root = path.resolve(import.meta.dirname, "../..");
const key = "ledger-browser-test-signing-key";
const profile = { user_id: "ledger-browser-user", user_email: "ledger@example.test", display: "Ledger Test User" };
let directory;
let binary;

async function reservePort() {
  const server = createServer();
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
function sessionToken() {
  const encode = value => Buffer.from(JSON.stringify(value)).toString("base64url");
  const data = `${encode({ alg: "HS256", typ: "JWT" })}.${encode({ iss: "tauth", tenant_id: "mprlab", user_id: profile.user_id, iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 3600 })}`;
  return `${data}.${createHmac("sha256", key).update(data).digest("base64url")}`;
}

test.beforeAll(async () => {
  directory = await mkdtemp(path.join(os.tmpdir(), "ledger-shared-ui-"));
  binary = path.join(directory, "ledger");
  execFileSync("go", ["build", "-o", binary, "./cmd/credit"], { cwd: root, timeout: 30_000 });
});
test.afterAll(async () => { if (directory) await rm(directory, { recursive: true, force: true }); });

for (const width of [390, 1280]) {
  test(`real shared UI authenticates and recovers Ledger requests at ${width}px`, async ({ page, context }) => {
    const port = await reservePort();
    const baseURL = `http://127.0.0.1:${port}`;
    const config = path.join(directory, `config-${width}.yml`);
    await writeFile(config, `service:
  database_url: "sqlite://${directory}/${width}.db"
  grpc_listen_addr: "127.0.0.1:0"
  http_listen_addr: "127.0.0.1:${port}"
auth:
  jwt_signing_key: "${key}"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
  public_origin: "${baseURL}"
ui:
  description: "Ledger browser test"
  tauth_url: "${baseURL}"
  google_client_id: "google-client-id"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  session_path: "/auth/session"
`);
    const child = spawn(binary, ["--config", config], { cwd: root, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", chunk => { output += chunk; });
    child.stderr.on("data", chunk => { output += chunk; });
    try {
      await expect.poll(async () => {
        if (child.exitCode !== null) throw new Error(`ledger_server_exit:${output}`);
        try { return (await fetch(`${baseURL}/healthz`)).status; } catch { return 0; }
      }).toBe(200);
      await installSharedUI(context);
      const alpine = await readFile(path.join(root, "web/node_modules/alpinejs/dist/module.esm.js"));
      await context.route("https://cdn.jsdelivr.net/npm/alpinejs@3.17.1/dist/module.esm.js", route => route.fulfill({ body: alpine, contentType: "text/javascript" }));
      // The production Pages profile selects fixed origins. This test assigns its temporary origin at that boundary.
      await context.route(`${baseURL}/assets/ledger/js/profile.js`, route => route.fulfill({ body: `export const BROWSER_PROFILE={apiOrigin:${JSON.stringify(baseURL)}};`, contentType: "text/javascript" }));
      let active = false;
      let restores = 0;
      let logins = 0;
      await context.route(`${baseURL}/auth/**`, async route => {
        const request = route.request();
        const pathname = new URL(request.url()).pathname;
        expect(request.headers()["x-tauth-tenant"]).toBe("mprlab");
        if (pathname === "/auth/nonce") {
          expect(request.method()).toBe("POST");
          return route.fulfill({ json: { nonce: "ledger-browser-nonce" } });
        }
        if (pathname === "/auth/google") {
          expect(request.postDataJSON()).toEqual({ google_id_token: "ledger-browser-google-credential", nonce_token: "ledger-browser-nonce" });
          active = true;
          logins += 1;
          return route.fulfill({ json: profile, headers: { "Set-Cookie": `app_session=${sessionToken()}; Path=/; HttpOnly; SameSite=Lax` } });
        }
        if (pathname === "/auth/session") {
          restores += 1;
          return route.fulfill(active ? { json: profile } : { status: 204, body: "" });
        }
        if (pathname === "/auth/logout") {
          active = false;
          return route.fulfill({ status: 204, body: "", headers: { "Set-Cookie": "app_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0" } });
        }
        throw new Error(`unexpected_auth_request:${pathname}`);
      });
      await page.setViewportSize({ width, height: 900 });
      await page.goto(baseURL);
      await expect(page.locator('mpr-header[data-mpr-auth-status="unauthenticated"]')).toBeVisible();
      expect(await page.evaluate(() => Boolean(window.isSecureContext && navigator.locks && crypto.subtle))).toBe(true);
      await page.getByRole("button", { name: "Documentation", exact: true }).click();
      await expect(page.getByRole("link", { name: "Ledger API", exact: true })).toHaveAttribute("href", "https://github.com/tyemirov/ledger/blob/master/docs/api.md");
      await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "Sign in with Google" }).click();
      await expect(page.getByRole('region', { name: 'Ledger workspace' })).toBeVisible();
      await expect(page.locator('mpr-header[data-mpr-auth-status="authenticated"]')).toBeVisible();
      expect(logins).toBe(1);
      const beforeReload = restores;
      await page.reload();
      await expect(page.getByRole('region', { name: 'Ledger workspace' })).toBeVisible();
      expect(restores).toBeGreaterThan(beforeReload);
      const attempts = [];
      const rejected = new Set();
      await page.route(`${baseURL}/api/tenants*`, async route => {
        const method = route.request().method();
        attempts.push(method);
        if (!rejected.has(method)) {
          rejected.add(method);
          return route.fulfill({ status: 401, json: { error: { message: "Expired session" } } });
        }
        return route.continue();
      });
      const result = await page.evaluate(async () => {
        const client = await import("/assets/ledger/js/client.js");
        const signal = new AbortController().signal;
        const before = await client.listTenants("", signal);
        const tenant = await client.createTenant("Recovery test", "browser-recovery-operation", signal);
        const after = await client.listTenants("", signal);
        return { before: before.tenants.length, after: after.tenants.length, name: tenant.name };
      });
      expect(result).toEqual({ before: 0, after: 1, name: "Recovery test" });
      expect(attempts).toEqual(["GET", "GET", "POST", "POST", "GET"]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.locator('mpr-user [data-mpr-user="trigger"]').click();
      await page.locator('mpr-user [data-mpr-user="logout"]').click();
      await expect(page.getByRole("button", { name: "Sign in with Google" })).toBeVisible();
      await expect(page.getByRole('region', { name: 'Ledger workspace' })).toBeHidden();
      expect(active).toBe(false);
      expect((await context.request.get(`${baseURL}/api/user-account`)).status()).toBe(401);
    } finally {
      if (child.exitCode === null) {
        const stopped = once(child, "exit");
        child.kill("SIGTERM");
        await stopped;
      }
    }
  });
}
