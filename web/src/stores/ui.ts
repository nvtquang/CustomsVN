import { create } from "zustand";
import { persist } from "zustand/middleware";

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

/** Client-side UI state. Only the theme is remembered between visits. */
export const useUI = create<UIState>()(
  persist(
    (set) => ({
      theme: "system",
      sidebarOpen: false,
      panelChunk: null,
      panelQuote: null,
      setTheme: (theme) => set({ theme }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      openChunk: (panelChunk, quote) => set({ panelChunk, panelQuote: quote ?? null }),
    }),
    { name: "customsvn-ui", partialize: (s) => ({ theme: s.theme }) },
  ),
);

export function applyTheme(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}
