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
    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Add multiple URLs' }).click();
    await expect(page).toHaveURL('/admin/monitors/bulk/new');
    await expect(page.getByRole('heading', { name: 'Add multiple Monitor URLs' })).toBeVisible();
    await expect(page.getByText("Each monitor's Name is set to its URL.")).toBeVisible();
  });

  test('creates monitors from comma and newline separated URLs with name equal to URL', async ({ page }) => {
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
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.locator('#notification_smtp_host').fill('smtp.example.com');
    await page.locator('#notification_smtp_port').fill('587');
    await page.locator('#notification_smtp_to').fill('ops@example.com');
    await page.getByRole('button', { name: 'Save' }).click();

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
        .getByRole('link', { name: url, exact: true })
        .first()
        .click();
      await page.getByRole('link', { name: 'Edit' }).click();
      await expect(page.locator('#notify_telegram')).toBeChecked();
      await expect(page.locator('#notify_smtp')).toBeChecked();
    }
  });

  test('rejects invalid URL without creating any monitors', async ({ page, request }) => {
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
