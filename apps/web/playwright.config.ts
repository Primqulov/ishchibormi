import { defineConfig, devices } from "@playwright/test";

// Every API request is intercepted by the fixture; SSR only reaches the local
// dummy server. Never reuse a server with an unknown API configuration.
const baseURL = "http://127.0.0.1:3105";
const apiURL = "http://127.0.0.1:4318";

export default defineConfig({
  testDir: "./tests/owner-flow",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  outputDir: "test-results/owner-flow",
  reporter: [["line"], ["html", { outputFolder: "playwright-report", open: "never" }]],
  use: {
    ...devices["Desktop Chrome"],
    channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
    baseURL,
    viewport: { width: 1440, height: 1100 },
    timezoneId: "Asia/Tashkent",
    locale: "uz-UZ",
    serviceWorkers: "block",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium" }],
  webServer: [
    {
      command: "node tests/owner-flow/mock-api-server.cjs",
      url: `${apiURL}/healthz`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "node node_modules/next/dist/bin/next dev -p 3105 -H 127.0.0.1",
      url: `${baseURL}/login`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        NEXT_PUBLIC_API_BASE: apiURL,
        NEXT_PUBLIC_SITE_URL: baseURL,
        NEXT_TELEMETRY_DISABLED: "1",
      },
    },
  ],
});
