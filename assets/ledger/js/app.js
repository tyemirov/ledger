// @ts-check

import "./alpine-runtime.js";
import { AUTH_STATES, COPY, EVENTS, MPR_UI, RESOURCE_STATES, SECTIONS } from "./constants.js";
import {
  createCredential,
  createTenant,
  listCredentials,
  listTenants,
  provisionUserAccount,
  revokeCredential,
} from "./client.js";

const EMPTY = "";
const RFC3339 = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/;

/** @param {string} value @returns {bigint} */
function timestampNanoseconds(value) {
  const match = RFC3339.exec(value);
  if (!match) throw new Error("ledger_tenant_created_at_invalid");
  const milliseconds = Date.parse(`${match[1]}${match[3]}`);
  if (!Number.isFinite(milliseconds)) throw new Error("ledger_tenant_created_at_invalid");
  const fraction = (match[2] || "").padEnd(9, "0") || "0";
  return BigInt(milliseconds) * 1_000_000n + BigInt(fraction);
}

/**
 * @template {object} State
 * @param {State & ThisType<any>} state
 * @returns {State}
 */
function workspaceState(state) {
  return state;
}

function ledgerWorkspace() {
  return workspaceState({
    authStates: AUTH_STATES,
    resourceStates: RESOURCE_STATES,
    sections: SECTIONS,
    authState: AUTH_STATES.LOADING,
    tenantState: RESOURCE_STATES.IDLE,
    credentialState: RESOURCE_STATES.IDLE,
    account: null,
    tenants: [],
    nextCursor: EMPTY,
    selectedTenantID: EMPTY,
    activeSection: SECTIONS.OVERVIEW,
    credentials: [],
    notice: EMPTY,
    noticeKind: "info",
    createTenantOpen: false,
    createTenantName: EMPTY,
    createTenantPending: false,
    tenantMutationController: null,
    generatedCredential: EMPTY,
    generatedCredentialID: EMPTY,
    credentialSaved: false,
    credentialCopied: false,
    createCredentialPending: false,
    revokeCredentialID: EMPTY,
    revokeCredentialPending: false,
    authVersion: 0,
    workspaceController: null,
    collectionController: null,
    credentialController: null,
    credentialMutationController: null,
    tenantDialogReturnFocus: null,
    credentialDialogReturnFocus: null,
    revokeDialogReturnFocus: null,

    get selectedTenant() {
      return this.tenants.find((item) => item.id === this.selectedTenantID) || null;
    },

    get selectedTenantCreatedAt() {
      return this.selectedTenant ? this.formatTime(this.selectedTenant.created_at) : EMPTY;
    },

    get activeCredentialCount() {
      return this.credentials.filter((item) => !item.revoked_at).length;
    },

    get integrationExample() {
      if (!this.selectedTenant) return EMPTY;
      return [
        `export LEDGER_TENANT_ID="${this.selectedTenant.id}"`,
        'export LEDGER_TENANT_CREDENTIAL="<tenant_credential>"',
        "",
        'grpcurl -H "authorization: Bearer ${LEDGER_TENANT_CREDENTIAL}" ' + "\\",
        '  -d "{\\"tenant_id\\":\\"${LEDGER_TENANT_ID}\\",\\"user_id\\":\\"<user_id>\\",\\"ledger_id\\":\\"default\\"}" ' + "\\",
        "  <ledger_grpc_endpoint> credit.v1.CreditService/GetBalance",
      ].join("\n");
    },

    init() {
      document.addEventListener(EVENTS.AUTHENTICATED, () => void this.openAuthenticatedWorkspace());
      document.addEventListener(EVENTS.UNAUTHENTICATED, () => this.setUnauthenticated());
      document.addEventListener(EVENTS.AUTH_STATUS_CHANGE, (event) => {
        const status = /** @type {CustomEvent<{status?: string}>} */ (event).detail?.status || EMPTY;
        if (status === AUTH_STATES.UNAUTHENTICATED) this.setUnauthenticated();
      });
      window.addEventListener("beforeunload", () => this.clearGeneratedCredential());
      void this.start();
    },

    async start() {
      try {
        await this.waitForMprUI();
        const status = this.readAuthStatus();
        if (status === AUTH_STATES.AUTHENTICATED) {
          await this.openAuthenticatedWorkspace();
        } else if (status === AUTH_STATES.UNAUTHENTICATED) {
          this.setUnauthenticated();
        }
      } catch (_error) {
        this.authState = AUTH_STATES.ERROR;
        this.setNotice("error", COPY.APP_ERROR);
        await this.dispatchWorkspaceReady();
      }
    },

    async waitForMprUI() {
      if (document.readyState === "loading") {
        await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
      }
      if (!window.MPRUI || typeof window.MPRUI.whenAutoOrchestrationReady !== "function") {
        throw new Error("ledger_mpr_ui_orchestration_missing");
      }
      await window.MPRUI.whenAutoOrchestrationReady();
    },

    readAuthStatus() {
      const header = document.getElementById(MPR_UI.HEADER_ID);
      const status = header?.getAttribute(MPR_UI.AUTH_STATUS_ATTRIBUTE)?.trim() || EMPTY;
      if (!status) throw new Error("ledger_mpr_ui_status_missing");
      return status;
    },

    async openAuthenticatedWorkspace() {
      if (this.authState === AUTH_STATES.AUTHENTICATED) {
        await this.dispatchWorkspaceReady();
        return;
      }
      if (this.authState === AUTH_STATES.LOADING && this.workspaceController) return;
      this.clearProtectedState();
      this.authVersion += 1;
      const version = this.authVersion;
      const controller = new AbortController();
      this.workspaceController = controller;
      this.authState = AUTH_STATES.LOADING;
      this.tenantState = RESOURCE_STATES.LOADING;
      try {
        this.account = await provisionUserAccount(controller.signal);
        const page = await listTenants(EMPTY, controller.signal);
        if (!this.canApply(version, controller)) return;
        this.mergeTenants(page.tenants);
        this.nextCursor = page.next_cursor;
        this.tenantState = this.tenants.length ? RESOURCE_STATES.READY : RESOURCE_STATES.EMPTY;
        this.authState = AUTH_STATES.AUTHENTICATED;
        if (this.tenants.length) {
          const requestedID = new URL(window.location.href).searchParams.get("tenant") || EMPTY;
          const selected = this.tenants.some((item) => item.id === requestedID) ? requestedID : this.tenants[0].id;
          await this.selectTenant(selected, false);
        }
      } catch (error) {
        if (!this.isAbort(error) && this.canApply(version, controller)) {
          this.authState = AUTH_STATES.ERROR;
          this.tenantState = RESOURCE_STATES.ERROR;
          this.setNotice("error", COPY.APP_ERROR);
        }
      } finally {
        if (this.workspaceController === controller) this.workspaceController = null;
        await this.dispatchWorkspaceReady();
      }
    },

    retryWorkspace() {
      this.authState = AUTH_STATES.UNAUTHENTICATED;
      void this.openAuthenticatedWorkspace();
    },

    setUnauthenticated() {
      this.clearProtectedState();
      this.authVersion += 1;
      this.authState = AUTH_STATES.UNAUTHENTICATED;
      this.setNotice("info", COPY.AUTH_REQUIRED);
    },

    clearProtectedState() {
      for (const name of ["workspaceController", "collectionController", "credentialController", "credentialMutationController", "tenantMutationController"]) {
        if (this[name]) this[name].abort();
        this[name] = null;
      }
      this.clearGeneratedCredential();
      this.account = null;
      this.tenants = [];
      this.nextCursor = EMPTY;
      this.selectedTenantID = EMPTY;
      this.credentials = [];
      this.tenantState = RESOURCE_STATES.IDLE;
      this.credentialState = RESOURCE_STATES.IDLE;
      this.createTenantOpen = false;
      this.revokeCredentialID = EMPTY;
      this.notice = EMPTY;
    },

    canApply(version, controller) {
      return this.authVersion === version && !controller.signal.aborted && this.readAuthStatus() === AUTH_STATES.AUTHENTICATED;
    },

    mergeTenants(items) {
      const tenantsByID = new Map(this.tenants.map((item) => [item.id, item]));
      for (const item of items) tenantsByID.set(item.id, item);
      this.tenants = [...tenantsByID.values()].sort((left, right) => {
        const leftCreatedAt = timestampNanoseconds(left.created_at);
        const rightCreatedAt = timestampNanoseconds(right.created_at);
        const createdAtOrder = leftCreatedAt < rightCreatedAt ? -1 : leftCreatedAt > rightCreatedAt ? 1 : 0;
        return createdAtOrder || left.id.localeCompare(right.id);
      });
    },

    async loadMoreTenants() {
      if (!this.nextCursor || this.collectionController) return;
      const version = this.authVersion;
      const cursor = this.nextCursor;
      const controller = new AbortController();
      this.collectionController = controller;
      try {
        const page = await listTenants(cursor, controller.signal);
        if (!this.canApply(version, controller) || cursor !== this.nextCursor) return;
        this.mergeTenants(page.tenants);
        this.nextCursor = page.next_cursor;
      } catch (error) {
        if (!this.isAbort(error)) this.setNotice("error", COPY.TENANTS_ERROR);
      } finally {
        if (this.collectionController === controller) this.collectionController = null;
      }
    },

    async selectTenant(tenantID, updateURL = true) {
      if (!this.tenants.some((item) => item.id === tenantID)) return;
      this.clearGeneratedCredential();
      if (this.credentialController) this.credentialController.abort();
      this.selectedTenantID = tenantID;
      this.activeSection = SECTIONS.OVERVIEW;
      this.credentials = [];
      if (updateURL) this.writeTenantURL(tenantID);
      const version = this.authVersion;
      const controller = new AbortController();
      this.credentialController = controller;
      this.credentialState = RESOURCE_STATES.LOADING;
      try {
        const items = await listCredentials(tenantID, controller.signal);
        if (!this.canApply(version, controller) || this.selectedTenantID !== tenantID) return;
        this.credentials = items;
        this.credentialState = items.length ? RESOURCE_STATES.READY : RESOURCE_STATES.EMPTY;
      } catch (error) {
        if (!this.isAbort(error) && this.selectedTenantID === tenantID) {
          this.credentialState = RESOURCE_STATES.ERROR;
          this.setNotice("error", COPY.CREDENTIALS_ERROR);
        }
      } finally {
        if (this.credentialController === controller) this.credentialController = null;
      }
    },

    writeTenantURL(tenantID) {
      const url = new URL(window.location.href);
      url.searchParams.set("tenant", tenantID);
      history.replaceState(null, EMPTY, url);
    },

    openCreateTenant(event) {
      this.tenantDialogReturnFocus = event.currentTarget;
      this.createTenantName = EMPTY;
      this.createTenantOpen = true;
      this.$nextTick(() => requestAnimationFrame(() => this.createTenantOpen && this.$refs.tenantName.focus()));
    },

    closeCreateTenant() {
      if (this.createTenantPending) return;
      this.createTenantOpen = false;
      this.$nextTick(() => this.tenantDialogReturnFocus?.focus());
    },

    async submitTenant() {
      const name = this.createTenantName.trim();
      if (!name || this.createTenantPending) return;
      const version = this.authVersion;
      const controller = new AbortController();
      this.tenantMutationController = controller;
      this.createTenantPending = true;
      try {
        const item = await createTenant(name, crypto.randomUUID(), controller.signal);
        if (!this.canApply(version, controller)) return;
        this.mergeTenants([item]);
        this.tenantState = RESOURCE_STATES.READY;
        this.createTenantOpen = false;
        this.setNotice("success", COPY.TENANT_CREATED);
        await this.selectTenant(item.id);
        this.$nextTick(() => this.tenantDialogReturnFocus?.focus());
      } catch (error) {
        if (!this.isAbort(error)) this.setNotice("error", this.errorMessage(error));
      } finally {
        if (this.tenantMutationController === controller) this.tenantMutationController = null;
        this.createTenantPending = false;
      }
    },

    async issueCredential(event) {
      if (!this.selectedTenant || this.createCredentialPending) return;
      this.clearGeneratedCredential();
      this.credentialDialogReturnFocus = event.currentTarget;
      const tenantID = this.selectedTenant.id;
      const version = this.authVersion;
      const controller = new AbortController();
      this.credentialMutationController = controller;
      this.createCredentialPending = true;
      try {
        const item = await createCredential(tenantID, crypto.randomUUID(), controller.signal);
        if (!this.canApply(version, controller) || this.selectedTenantID !== tenantID) return;
        this.credentials = [...this.credentials, { ...item, secret: EMPTY }];
        this.credentialState = RESOURCE_STATES.READY;
        this.generatedCredential = item.secret;
        this.generatedCredentialID = item.id;
        this.setNotice("success", COPY.CREDENTIAL_CREATED);
        this.$nextTick(() => requestAnimationFrame(() => this.generatedCredential && this.$refs.generatedCredentialDialog.focus()));
      } catch (error) {
        if (!this.isAbort(error)) this.setNotice("error", this.errorMessage(error));
      } finally {
        if (this.credentialMutationController === controller) this.credentialMutationController = null;
        this.createCredentialPending = false;
      }
    },

    async copyGeneratedCredential() {
      try {
        await navigator.clipboard.writeText(this.generatedCredential);
        this.credentialCopied = true;
        this.setNotice("success", COPY.COPY_COMPLETE);
      } catch (_error) {
        this.setNotice("error", COPY.COPY_FAILED);
      }
    },

    closeGeneratedCredential() {
      if (!this.credentialSaved) return;
      this.clearGeneratedCredential();
      this.$nextTick(() => this.credentialDialogReturnFocus?.focus());
    },

    clearGeneratedCredential() {
      this.generatedCredential = EMPTY;
      this.generatedCredentialID = EMPTY;
      this.credentialSaved = false;
      this.credentialCopied = false;
    },

    openRevokeCredential(credentialID, event) {
      this.revokeDialogReturnFocus = event.currentTarget;
      this.revokeCredentialID = credentialID;
      this.$nextTick(() => requestAnimationFrame(() => this.revokeCredentialID && this.$refs.revokeConfirm.focus()));
    },

    closeRevokeCredential() {
      if (this.revokeCredentialPending) return;
      this.revokeCredentialID = EMPTY;
      this.$nextTick(() => this.revokeDialogReturnFocus?.focus());
    },

    async confirmRevokeCredential() {
      const tenantID = this.selectedTenantID;
      const credentialID = this.revokeCredentialID;
      if (!tenantID || !credentialID || this.revokeCredentialPending) return;
      const version = this.authVersion;
      const controller = new AbortController();
      this.credentialMutationController = controller;
      this.revokeCredentialPending = true;
      try {
        const item = await revokeCredential(tenantID, credentialID, controller.signal);
        if (!this.canApply(version, controller) || this.selectedTenantID !== tenantID || this.revokeCredentialID !== credentialID) return;
        this.credentials = this.credentials.map((candidate) => candidate.id === item.id ? item : candidate);
        this.revokeCredentialID = EMPTY;
        this.setNotice("success", COPY.CREDENTIAL_REVOKED);
        this.$nextTick(() => this.revokeDialogReturnFocus?.focus());
      } catch (error) {
        if (!this.isAbort(error)) this.setNotice("error", this.errorMessage(error));
      } finally {
        if (this.credentialMutationController === controller) this.credentialMutationController = null;
        this.revokeCredentialPending = false;
      }
    },

    setSection(section) {
      this.clearGeneratedCredential();
      this.activeSection = section;
    },

    setNotice(kind, message) {
      this.noticeKind = kind;
      this.notice = message;
    },

    errorMessage(error) {
      return error instanceof Error && error.message ? error.message : COPY.APP_ERROR;
    },

    isAbort(error) {
      return error instanceof DOMException && error.name === "AbortError";
    },

    formatTime(value) {
      return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
    },

    async dispatchWorkspaceReady() {
      try {
        await this.waitForMprUI();
        document.dispatchEvent(new CustomEvent(EVENTS.WORKSPACE_READY));
      } catch (_error) {
        document.documentElement.setAttribute("data-ledger-transition-error", "true");
      }
    },
  });
}

window.Alpine.data("ledgerWorkspace", ledgerWorkspace);
window.Alpine.start();
document.documentElement.setAttribute("data-ledger-application", "ready");
