import type Alpine from "alpinejs";

declare global {
  interface Window {
    Alpine: typeof Alpine;
    MPRUI?: {
      whenAutoOrchestrationReady?: () => Promise<unknown>;
      authenticatedFetch?: (
        authTarget: object,
        input: RequestInfo | URL,
        init?: RequestInit,
        policy?: { mutationReplay?: "authorization-before-domain-work" },
      ) => Promise<Response>;
    };
  }
}

export {};
