import { test, expect, APIRequestContext } from '@playwright/test';

// E2e-тесты массового добавления мониторов (bulk): парсинг URL, флаги уведомлений, валидация.
// beforeEach задаёт check_interval_seconds и чистит таблицы мониторов.

/** POST к dev-only Playwright API; падает тест, если сервер вернул не 2xx. */
async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown>) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

/** Логин admin + проверка успешного входа (timeout 15s — Docker/воркер могут стартовать медленно). */
async function login(page: import('@playwright/test').Page, request: APIRequestContext) {
  await apiCall(request, 'clear-table', { table: 'users' });
  await apiCall(request, 'create-user', { username: 'admin', password: 'password123' });

  await page.goto('/login');
  await page.locator('#username').fill('admin');
  await page.locator('#password').fill('password123');
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page.getByText('Invalid username or password')).toHaveCount(0);
  await expect(page).toHaveURL('/admin/', { timeout: 15000 });
}

/** Экранирование URL для использования в RegExp (точки и слэши в hostname). */
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test.describe('Bulk add monitors', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'app_settings' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO app_settings (key, value) VALUES ('check_interval_seconds', '60')`,
    });
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('opens bulk form from monitors list', async ({ page }) => {
    // Навигация со списка мониторов на bulk/new.
    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Add multiple URLs' }).click();
    await expect(page).toHaveURL('/admin/monitors/bulk/new');
    await expect(page.getByRole('heading', { name: 'Add multiple Monitor URLs' })).toBeVisible();
    await expect(page.getByText("Each monitor's Name is set to its URL.")).toBeVisible();
    await expect(page.getByLabel('Skip URLs that already exist')).toBeVisible();
  });

  test('creates monitors from comma and newline separated URLs with name equal to URL', async ({ page }) => {
    // Три URL через запятую и перевод строки — все должны появиться в таблице с name=url.
    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill(
      'https://bulk-a.example.com, https://bulk-b.example.com\nhttps://bulk-c.example.com',
    );
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    for (const url of [
      'https://bulk-a.example.com',
      'https://bulk-b.example.com',
      'https://bulk-c.example.com',
    ]) {
      const row = page.getByRole('row', { name: new RegExp(escapeRegExp(url)) });
      await expect(row).toBeVisible();
      await expect(row.getByRole('link', { name: url, exact: true }).first()).toBeVisible();
    }
  });

  test('applies notification flags to every created monitor', async ({ page }) => {
    // Сначала сохраняем credentials в Settings — иначе чекбоксы notify недоступны.
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.locator('#notification_smtp_host').fill('smtp.example.com');
    await page.locator('#notification_smtp_port').fill('587');
    await page.locator('#notification_smtp_to').fill('ops@example.com');
    await page.getByRole('button', { name: 'Save' }).click();

    // Bulk create с notify_telegram и notify_smtp — проверяем на форме Edit каждого монитора.
    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill('https://bulk-notify-a.example.com\nhttps://bulk-notify-b.example.com');
    await page.locator('#notify_telegram').check();
    await page.locator('#notify_smtp').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');

    for (const url of ['https://bulk-notify-a.example.com', 'https://bulk-notify-b.example.com']) {
      await page.goto('/admin/monitors');
      await page
        .getByRole('row', { name: new RegExp(escapeRegExp(url)) })
        .getByRole('link', { name: 'Edit' })
        .click();
      await expect(page.locator('#notify_telegram')).toBeChecked();
      await expect(page.locator('#notify_smtp')).toBeChecked();
    }
  });

  test('rejects invalid URL without creating any monitors', async ({ page, request }) => {
    // Одна невалидная строка — откат всей bulk-операции, форма сохраняет ввод.
    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill('https://valid-bulk.example.com\nnot-a-url');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors/bulk');
    await expect(page.locator('.alert-danger')).toContainText('not-a-url');
    await expect(page.locator('#urls')).toHaveValue('https://valid-bulk.example.com\nnot-a-url');

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('0');
  });
});
