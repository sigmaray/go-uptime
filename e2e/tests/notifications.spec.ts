import { test, expect, APIRequestContext } from '@playwright/test';

async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown>) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

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

test.describe('Notifications', () => {
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

  test('shows notification settings fields', async ({ page }) => {
    await page.goto('/admin/settings');
    await expect(page.locator('#notification_telegram_url')).toBeVisible();
    await expect(page.locator('#notification_smtp_host')).toBeVisible();
    await expect(page.locator('#notification_smtp_to')).toBeVisible();
  });

  test('saves telegram and smtp settings', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.locator('#notification_smtp_host').fill('smtp.example.com');
    await page.locator('#notification_smtp_port').fill('587');
    await page.locator('#notification_smtp_to').fill('ops@example.com');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page).toHaveURL('/admin/settings');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.locator('#notification_telegram_url')).toHaveValue('telegram://token@telegram?channels=123');
    await expect(page.locator('#notification_smtp_host')).toHaveValue('smtp.example.com');
    await expect(page.locator('#notification_smtp_to')).toHaveValue('ops@example.com');
  });

  test('disables monitor notification checkboxes without credentials', async ({ page }) => {
    await page.goto('/admin/monitors/new');
    await expect(page.locator('#notify_telegram')).toBeDisabled();
    await expect(page.locator('#notify_smtp')).toBeDisabled();
    await expect(page.getByText('Configure Telegram or SMTP credentials in')).toBeVisible();
  });

  test('enables monitor notification checkboxes when credentials exist', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.locator('#notification_smtp_host').fill('smtp.example.com');
    await page.locator('#notification_smtp_to').fill('ops@example.com');
    await page.getByRole('button', { name: 'Save' }).click();

    await page.goto('/admin/monitors/new');
    await expect(page.locator('#notify_telegram')).toBeEnabled();
    await expect(page.locator('#notify_smtp')).toBeEnabled();
    await expect(page.getByText('Telegram is not configured in system settings.')).toHaveCount(0);
  });

  test('saves monitor notification preferences', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.getByRole('button', { name: 'Save' }).click();

    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://notify.example.com');
    await page.locator('#notify_telegram').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await page.getByRole('cell', { name: 'notify.example.com', exact: true }).getByRole('link').click();
    await page.getByRole('link', { name: 'Edit' }).click();
    await expect(page.locator('#notify_telegram')).toBeChecked();
  });
});

test.describe('Monitor uptime display', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('shows separated uptime blocks with 1h on monitor detail', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://uptime-detail.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `UPDATE monitor_urls SET created_at = NOW() - INTERVAL '2 hours'
              WHERE url = 'https://uptime-detail.example.com'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO stat_minutely (monitor_url_id, bucket_at, up_seconds, total_seconds)
              SELECT id, date_trunc('minute', NOW()), 3000, 3600
              FROM monitor_urls WHERE url = 'https://uptime-detail.example.com'`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('cell', { name: 'uptime-detail.example.com', exact: true }).getByRole('link').click();

    const blocks = page.locator('.uptime-stats__item');
    await expect(blocks).toHaveCount(4);
    await expect(blocks.filter({ hasText: '1h' })).toBeVisible();
    await expect(blocks.filter({ hasText: '24h' })).toBeVisible();
    await expect(blocks.filter({ hasText: '30d' })).toBeVisible();
    await expect(blocks.filter({ hasText: '1y' })).toBeVisible();
    await expect(blocks.first()).toHaveCSS('border-style', 'solid');
    await expect(page.getByText('83.33%')).toBeVisible();
  });
});
