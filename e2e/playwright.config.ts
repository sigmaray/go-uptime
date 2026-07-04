import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html']] : 'html',
  use: {
    baseURL: 'http://localhost:18081',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'bash scripts/start-test-server.sh',
    url: 'http://localhost:18081/health',
    reuseExistingServer: false,
    timeout: 300 * 1000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
