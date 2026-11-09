import { describe, expect, it } from "vitest";
import { createUIStore } from "./ui";

function memoryStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return { data, getItem: (k: string) => data[k] ?? null, setItem: (k: string, v: string) => void (data[k] = v) };
}

describe("UI store", () => {
  it("starts from the saved theme, in the format index.html reads before React loads", () => {
    const storage = memoryStorage({ "customsvn-ui": JSON.stringify({ state: { theme: "dark" }, version: 0 }) });
    expect(createUIStore(storage).getState().theme).toBe("dark");
    expect(createUIStore(memoryStorage({ "customsvn-ui": "not json" })).getState().theme).toBe("system");
    expect(createUIStore(undefined).getState().theme).toBe("system");
  });

  it("remembers only the theme", () => {
    const storage = memoryStorage();
    const store = createUIStore(storage);
    store.getState().setSidebarOpen(true);
    expect(storage.data["customsvn-ui"]).toBeUndefined();
    store.getState().setTheme("light");
    expect(JSON.parse(storage.data["customsvn-ui"]).state).toEqual({ theme: "light" });
  });

  it("notifies subscribers until they unsubscribe", () => {
    const store = createUIStore(undefined);
    let calls = 0;
    const off = store.subscribe(() => calls++);
    store.getState().openChunk("c1", "quote");
    expect(store.getState()).toMatchObject({ panelChunk: "c1", panelQuote: "quote" });
    store.getState().openChunk(null);
    expect(store.getState().panelQuote).toBeNull();
    off();
    store.getState().setSidebarOpen(true);
    expect(calls).toBe(2);
  });
});
