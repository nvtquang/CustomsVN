import { useSyncExternalStore } from "react";

export type Theme = "light" | "dark" | "system";

interface UIState {
  theme: Theme;
  sidebarOpen: boolean;
  panelChunk: string | null;
  /** A verified quote to highlight in the panel. */
  panelQuote: string | null;
  setTheme: (t: Theme) => void;
  setSidebarOpen: (open: boolean) => void;
  openChunk: (id: string | null, quote?: string) => void;
}

type Storage = { getItem(key: string): string | null; setItem(key: string, value: string): void };

// index.html reads this key before React loads, to paint the right theme at once; keep the shape.
const KEY = "customsvn-ui";

function savedTheme(storage: Storage | undefined): Theme {
  try {
    const t = JSON.parse(storage?.getItem(KEY) || "{}").state?.theme;
    return t === "light" || t === "dark" ? t : "system";
  } catch {
    return "system";
  }
}

/** Client-side UI state. Only the theme is remembered between visits. */
export function createUIStore(storage: Storage | undefined) {
  const listeners = new Set<() => void>();
  const set = (patch: Partial<UIState>) => {
    state = { ...state, ...patch };
    if ("theme" in patch) {
      try {
        storage?.setItem(KEY, JSON.stringify({ state: { theme: state.theme }, version: 0 }));
      } catch {
        // Storage full or blocked: the theme still applies for this visit.
      }
    }
    listeners.forEach((l) => l());
  };
  let state: UIState = {
    theme: savedTheme(storage),
    sidebarOpen: false,
    panelChunk: null,
    panelQuote: null,
    setTheme: (theme) => set({ theme }),
    setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
    openChunk: (panelChunk, quote) => set({ panelChunk, panelQuote: quote ?? null }),
  };
  return {
    getState: () => state,
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

function browserStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined; // no window (tests) or storage blocked
  }
}

const store = createUIStore(browserStorage());

/** Reads one slice of the UI state; the component re-renders when that slice changes. */
export function useUI<T>(select: (s: UIState) => T): T {
  return useSyncExternalStore(store.subscribe, () => select(store.getState()));
}

export function applyTheme(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}
