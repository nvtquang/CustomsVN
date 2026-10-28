import { defineConfig, devices } from "@playwright/test";

// End-to-end tests of the main flows against a running customsvn serve with
// a loaded corpus. They log in as the admin, point the LLM settings at the
// fake model in e2e/mock-llm.mjs and create a test user, so run them against
// a test instance, never a real one. See e2e/README.md.
export default defineConfig({
  testDir: "e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  globalSetup: "./e2e/global-setup.ts",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:18080",
    locale: "vi-VN",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "desktop",
      testIgnore: /mobile\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], storageState: "e2e/.auth/admin.json" },
    },
    {
      name: "mobile",
      testMatch: /mobile\.spec\.ts/,
      use: { ...devices["Pixel 7"], storageState: "e2e/.auth/user.json" },
    },
  ],
  webServer: {
    command: "node e2e/mock-llm.mjs 18090",
    port: 18090,
    reuseExistingServer: true,
  },
});
