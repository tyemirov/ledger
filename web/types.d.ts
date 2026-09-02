import type Alpine from "alpinejs";

declare global {
  interface Window {
    Alpine: typeof Alpine;
    MPRUI?: {
      whenAutoOrchestrationReady?: () => Promise<unknown>;
    };
  }
}

export {};
