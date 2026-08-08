import { defineConfig, devices } from '@playwright/test';

// Конфигурация Playwright для e2e-тестов go-uptime.
// Перед тестами поднимается Docker-стек через scripts/start-test-server.sh (webServer ниже).
export default defineConfig({
  // Каталог со spec-файлами относительно e2e/.
  testDir: './tests',
  // Тесты в одном worker идут последовательно — общая БД, нельзя параллелить без изоляции.
  fullyParallel: false,
  // В CI запрет test.only — иначе пайплайн молча пропустит остальные тесты.
  forbidOnly: !!process.env.CI,
  // В CI два повтора при флаке; локально без retries для быстрой обратной связи.
  retries: process.env.CI ? 2 : 0,
  // Один worker: один процесс браузера, одна тестовая БД, без гонок за данные.
  workers: 1,
  // CI: аннотации GitHub Actions + HTML-отчёт; локально только HTML.
  reporter: process.env.CI ? [['github'], ['html']] : 'html',
  use: {
    // baseURL — порт 18081 проброшен из Docker app на хост; page.goto('/login') идёт сюда.
    baseURL: 'http://localhost:18081',
    // Trace сохраняется только при retry — не раздувает артефакты на зелёных прогонах.
    trace: 'on-first-retry',
  },
  projects: [
    {
      // Один браузерный проект — Desktop Chrome достаточен для smoke/regression UI.
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    // Скрипт поднимает postgres + app в compose; Playwright ждёт готовности по url ниже.
    command: 'bash scripts/start-test-server.sh',
    // Healthcheck приложения — сигнал, что можно начинать тесты.
    url: 'http://localhost:18081/health',
    // Каждый прогон npx playwright test — свежий контейнер, без переиспользования старого сервера.
    reuseExistingServer: false,
    // Сборка образа + миграции + старт app могут занять несколько минут.
    timeout: 300 * 1000,
    // Логи compose в stdout/stderr Playwright — видны при падении webServer.
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
