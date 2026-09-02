// @ts-check

import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";

const repositoryRoot = path.resolve(import.meta.dirname, "../..");
const alpineModule = await readFile(path.join(import.meta.dirname, "../node_modules/alpinejs/dist/module.esm.js"), "utf8");
const tenantOne = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf801", name: "Primary", created_at: "2026-09-01T20:01:00Z" });
const tenantTwo = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf802", name: "Sandbox", created_at: "2026-09-01T20:02:00Z" });
const tenantThree = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf803", name: "Analytics", created_at: "2026-09-01T20:03:00Z" });
const tenantFour = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf804", name: "Archive", created_at: "2026-09-01T20:02:00Z" });
const credentialOne = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf811", tenant_id: tenantOne.id, created_at: "2026-09-01T20:04:00Z" });
const credentialTwo = Object.freeze({ id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf812", tenant_id: tenantThree.id, created_at: "2026-09-01T20:05:00Z" });
const generatedSecret = `ledger_${credentialTwo.id}_MDEyMzQ1Njc4OWFiY2RlZmdoaWprbG1ub3BxcnN0dXY`;

let processHandle;
let temporaryDirectory;
let baseURL;

test.beforeAll(async () => {
  temporaryDirectory = await mkdtemp(path.join(os.tmpdir(), "ledger-browser-"));
  const port = await reservePort();
  baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(temporaryDirectory, "config.yml");
  await writeFile(configPath, `
service:
  database_url: "sqlite://${path.join(temporaryDirectory, "ledger.db")}"
  grpc_listen_addr: "127.0.0.1:0"
  http_listen_addr: "127.0.0.1:${port}"
auth:
  jwt_signing_key: "browser-test-signing-key"
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
  processHandle = spawn("go", ["run", "./cmd/credit", "--config", configPath], {
    cwd: repositoryRoot,
    stdio: "ignore",
  });
  await waitForHealth(`${baseURL}/healthz`);
});

test.afterAll(async () => {
  if (processHandle && processHandle.exitCode === null) {
    processHandle.kill("SIGTERM");
    await new Promise((resolve) => processHandle.once("exit", resolve));
  }
  if (temporaryDirectory) await rm(temporaryDirectory, { recursive: true, force: true });
});

test.beforeEach(async ({ page }) => {
  await page.route("https://cdn.jsdelivr.net/npm/alpinejs@3.17.1/dist/module.esm.js", (route) => route.fulfill({ contentType: "text/javascript", body: alpineModule }));
  await page.route("https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css", (route) => route.fulfill({ contentType: "text/css", body: "" }));
  await page.route("https://cdn.jsdelivr.net/npm/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js", (route) => route.fulfill({ contentType: "text/javascript", body: "" }));
  await page.route("https://accounts.google.com/gsi/client", (route) => route.fulfill({ contentType: "text/javascript", body: "" }));
  await page.route("https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js", (route) => route.fulfill({
    contentType: "text/javascript",
    body: `window.MPRUI={whenAutoOrchestrationReady:()=>new Promise((resolve)=>{const ready=()=>{const header=document.getElementById("ledger-header");if(!header.hasAttribute("data-mpr-auth-status"))header.setAttribute("data-mpr-auth-status","unauthenticated");resolve();};if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",ready,{once:true});else ready();})};`,
  }));
});

test("uses the canonical MPR shell and sends no protected request before authentication", async ({ page }) => {
  let protectedRequests = 0;
  await page.route(`${baseURL}/api/**`, (route) => {
    protectedRequests += 1;
    return route.abort();
  });
  await page.goto(baseURL);
  await expect.poll(() => page.locator("html").getAttribute("data-ledger-application")).toBe("ready");
  await expect(page.getByRole("heading", { name: "Sign in to Ledger" })).toBeVisible();
  expect(protectedRequests).toBe(0);

  const html = await (await fetch(baseURL)).text();
  expect(html).toContain('data-config-url="/config-ui.yaml"');
  expect(html).toContain("mpr-ui-config.js");
  expect(html).toContain("mpr-ui@latest/mpr-ui.js");
  expect(html).not.toContain("tauth.js");
  expect(html).not.toContain("tauth-login-path");
  const configuration = await (await fetch(`${baseURL}/config-ui.yaml`)).text();
  expect(configuration).toContain('tenantId: "mprlab"');
  expect(configuration).toContain(`- "${baseURL}"`);
});

test("aligns the public shell controls on shared vertical edges", async ({ page }) => {
  await page.unroute("https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css");
  await page.unroute("https://cdn.jsdelivr.net/npm/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js");
  await page.unroute("https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(baseURL);
  await expect.poll(() => page.evaluate(() => Boolean(customElements.get("mpr-header") && customElements.get("mpr-footer")))).toBe(true);

  const brand = page.getByRole("link", { name: "Ledger workspace" });
  const signIn = page.getByRole("button", { name: "Sign in", exact: true });
  const privacy = page.getByRole("link", { name: "Privacy • Terms", exact: true });
  const documentation = page.getByRole("button", { name: "Documentation", exact: true });
  await expect(brand).toBeVisible();
  await expect(signIn).toBeVisible();
  await expect(privacy).toBeVisible();
  await expect(documentation).toBeVisible();

  const [brandBox, signInBox, privacyBox, documentationBox] = await Promise.all([
    brand.boundingBox(),
    signIn.boundingBox(),
    privacy.boundingBox(),
    documentation.boundingBox(),
  ]);
  expect(brandBox).not.toBeNull();
  expect(signInBox).not.toBeNull();
  expect(privacyBox).not.toBeNull();
  expect(documentationBox).not.toBeNull();
  if (!brandBox || !signInBox || !privacyBox || !documentationBox) throw new Error("shell_control_not_rendered");
  expect(Math.abs(brandBox.x - privacyBox.x)).toBeLessThanOrEqual(1);
  expect(Math.abs(signInBox.x + signInBox.width - documentationBox.x - documentationBox.width)).toBeLessThanOrEqual(1);

  await page.evaluate(() => {
    const mprUI = Reflect.get(window, "MPRUI");
    const header = document.getElementById("ledger-header");
    mprUI.testing.authenticate(header, {
      user_id: "browser-layout-user",
      display: "Browser Layout User",
      given_name: "Browser",
      user_email: "browser-layout@example.test",
    });
  });
  const userControl = page.locator("mpr-user");
  await expect(userControl).toBeVisible();
  const userControlBox = await userControl.boundingBox();
  expect(userControlBox).not.toBeNull();
  if (!userControlBox) throw new Error("user_control_not_rendered");
  expect(Math.abs(userControlBox.x + userControlBox.width - documentationBox.x - documentationBox.width)).toBeLessThanOrEqual(1);

  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});

test("shows one tenant creation action for an empty collection", async ({ page }) => {
  await page.route(`${baseURL}/api/**`, (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === "PUT" && url.pathname === "/api/user-account") {
      return json(route, { user_account: { id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf810", created_at: "2026-09-01T20:00:00Z" } });
    }
    if (request.method() === "GET" && url.pathname === "/api/tenants") return json(route, { tenants: [] });
    return json(route, { error: { code: "not_found", message: "Not found", request_id: "request" } }, 404);
  });

  await page.goto(baseURL);
  await page.evaluate(() => {
    const header = document.getElementById("ledger-header");
    header?.setAttribute("data-mpr-auth-status", "authenticated");
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated"));
  });
  await expect(page.getByText("Create your first tenant to start a Ledger integration.")).toBeVisible();
  const visibleCreateButtons = page.getByRole("button", { name: "Create tenant", exact: true }).filter({ visible: true });
  await expect(visibleCreateButtons).toHaveCount(1);

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(visibleCreateButtons).toHaveCount(1);
  await expect(visibleCreateButtons).toHaveText("Create tenant");
});

test("manages tenants and one-time credentials through the authenticated workspace", async ({ page }) => {
  /** @type {Array<{method: string, url: string, idempotencyKey: string, body: unknown}>} */
  const calls = [];
  await page.route(`${baseURL}/api/**`, async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    calls.push({
      method: request.method(),
      url: url.pathname + url.search,
      idempotencyKey: request.headers()["idempotency-key"] || "",
      body: request.postData() ? request.postDataJSON() : null,
    });
    if (request.method() === "PUT" && url.pathname === "/api/user-account") {
      return json(route, { user_account: { id: "0196f0ec-3e80-7a54-bd2b-56cfe90bf810", created_at: "2026-09-01T20:00:00Z" } });
    }
    if (request.method() === "GET" && url.pathname === "/api/tenants") {
      if (url.searchParams.get("cursor") === "cursor-2") return json(route, { tenants: [tenantFour, tenantThree] });
      return json(route, { tenants: [tenantOne, tenantTwo], next_cursor: "cursor-2" });
    }
    if (request.method() === "POST" && url.pathname === "/api/tenants") {
      return json(route, { tenant: tenantThree }, 201);
    }
    if (request.method() === "GET" && url.pathname.endsWith("/credentials")) {
      const tenantID = url.pathname.split("/")[3];
      return json(route, { credentials: tenantID === tenantOne.id ? [credentialOne] : [] });
    }
    if (request.method() === "POST" && url.pathname === `/api/tenants/${tenantThree.id}/credentials`) {
      return json(route, { credential: { ...credentialTwo, secret: generatedSecret } }, 201);
    }
    if (request.method() === "DELETE" && url.pathname === `/api/tenants/${tenantThree.id}/credentials/${credentialTwo.id}`) {
      return json(route, { credential: { ...credentialTwo, revoked_at: "2026-09-01T21:00:00Z" } });
    }
    return json(route, { error: { code: "not_found", message: "Not found", request_id: "request" } }, 404);
  });

  await page.goto(baseURL);
  expect(calls).toHaveLength(0);
  await page.evaluate(() => {
    const header = document.getElementById("ledger-header");
    header?.setAttribute("data-mpr-auth-status", "authenticated");
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:authenticated"));
  });
  await expect(page.getByRole("button", { name: "Primary" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Primary" })).toBeVisible();
  expect(calls[0].method).toBe("PUT");
  expect(calls[1].url).toContain("/api/tenants?limit=50");

  const createButton = page.getByRole("button", { name: "Create tenant" }).first();
  await createButton.focus();
  await createButton.click();
  await expect(page.getByLabel("Tenant name")).toBeFocused();
  await page.getByLabel("Tenant name").fill("Analytics");
  await page.getByRole("button", { name: "Create tenant", exact: true }).last().click();
  await expect(page.getByRole("heading", { name: "Analytics" })).toBeVisible();
  await expect(createButton).toBeFocused();
  await expect(page.locator(".tenant-row strong")).toHaveText(["Primary", "Sandbox", "Analytics"]);
  const tenantCreation = calls.find((call) => call.method === "POST" && call.url === "/api/tenants");
  expect(tenantCreation).toMatchObject({ idempotencyKey: expect.any(String), body: { name: "Analytics" } });
  expect(tenantCreation?.idempotencyKey).not.toBe("");
  await expect.poll(() => new URL(page.url()).searchParams.get("tenant")).toBe(tenantThree.id);

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.locator(".tenant-row strong")).toHaveText(["Primary", "Sandbox", "Archive", "Analytics"]);
  expect(calls.find((call) => call.method === "GET" && call.url.includes("cursor=cursor-2"))).toBeTruthy();

  await page.getByRole("button", { name: "Credentials" }).click();
  const createCredentialButton = page.getByRole("button", { name: "Create credential" });
  await createCredentialButton.click();
  const secretDialog = page.getByRole("dialog", { name: "Save this credential" });
  await expect(secretDialog).toBeVisible();
  await expect(secretDialog).toBeFocused();
  await expect(secretDialog.getByText(generatedSecret)).toBeVisible();
  const credentialCreation = calls.find((call) => call.method === "POST" && call.url.endsWith("/credentials"));
  expect(credentialCreation?.idempotencyKey).not.toBe("");
  const exposure = await page.evaluate((secret) => ({
    url: location.href.includes(secret),
    local: Object.values(localStorage).some((value) => value.includes(secret)),
    session: Object.values(sessionStorage).some((value) => value.includes(secret)),
    attributes: [...document.querySelectorAll("*")].some((element) => [...element.attributes].some((attribute) => attribute.value.includes(secret))),
  }), generatedSecret);
  expect(exposure).toEqual({ url: false, local: false, session: false, attributes: false });
  await secretDialog.getByLabel("I saved this credential.").check();
  await secretDialog.getByRole("button", { name: "Done" }).click();
  await expect(secretDialog).toBeHidden();
  await expect(createCredentialButton).toBeFocused();

  const revokeButton = page.getByRole("button", { name: "Revoke" });
  await revokeButton.click();
  const revokeDialog = page.getByRole("dialog", { name: "Revoke credential?" });
  await expect(revokeDialog).toBeFocused();
  await expect(revokeDialog.getByText(tenantThree.id)).toBeVisible();
  await expect(revokeDialog.getByText(credentialTwo.id)).toBeVisible();
  await revokeDialog.getByRole("button", { name: "Revoke credential" }).click();
  await expect(page.locator(".credential-list strong")).toHaveText("Revoked");
  await expect(revokeButton).toHaveText("Revoked");
  await expect(revokeButton).toHaveAttribute("aria-disabled", "true");
  await expect(revokeButton).toBeFocused();

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".mobile-tenant-control")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.setViewportSize({ width: 900, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.setViewportSize({ width: 1280, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

  await page.evaluate(() => {
    const header = document.getElementById("ledger-header");
    header?.setAttribute("data-mpr-auth-status", "unauthenticated");
    document.dispatchEvent(new CustomEvent("mpr-ui:auth:unauthenticated"));
  });
  await expect(page.getByRole("heading", { name: "Sign in to Ledger" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Analytics" })).toHaveCount(0);
});

async function reservePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("port_reservation_failed");
  const port = address.port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}

async function waitForHealth(url) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch (_error) {
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  throw new Error("ledger_server_start_failed");
}

function json(route, body, status = 200) {
  return route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}
