import { defineConfig } from "@playwright/test";

const localServer = ["http:/", "/127.0.0.1:4173"].join("");

export default defineConfig({
  expect: {
    toHaveScreenshot: {
      animations: "disabled",
      caret: "hide",
      maxDiffPixelRatio: 0.001,
      scale: "css",
    },
  },
  fullyParallel: false,
  reporter: "line",
  testDir: "./visual-tests",
  use: {
    baseURL: localServer,
    browserName: "chromium",
    channel: "chromium",
    colorScheme: "light",
    headless: true,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
  },
  webServer: {
    command: "./node_modules/.bin/vite --host 127.0.0.1 --port 4173",
    reuseExistingServer: false,
    timeout: 30_000,
    url: `${localServer}/visual.html?state=partial-overview`,
  },
  workers: 1,
});
