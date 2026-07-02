import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: '.',
  timeout: 45000,
  retries: 0,
  use: {
    baseURL: 'http://localhost:5173',
    screenshot: 'on',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        launchOptions: {
          args: ['--no-sandbox'],
          executablePath: process.env.CHROMIUM_PATH || undefined,
        },
      },
    },
  ],
  snapshotDir: '../screenshots',
})
