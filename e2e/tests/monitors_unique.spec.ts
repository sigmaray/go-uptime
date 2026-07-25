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

test.describe('Monitor URL uniqueness', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('rejects creating a monitor with a duplicate URL', async ({ page, request }) => {
    const url = 'https://duplicate-url.example.com';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('First');
    await page.locator('#url').fill(url);
    await page.getByRole('button', { name: 'Create' }).click();
    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Second');
    await page.locator('#url').fill(url);
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.locator('.alert-danger')).toContainText('A monitor with this URL already exists');
    await expect(page.locator('#url')).toHaveValue(url);
    await expect(page.locator('#name')).toHaveValue('Second');

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls WHERE url = '${url}'`,
    });
    expect(result.rows[0][0]).toBe('1');
  });

  test('rejects updating a monitor URL to one that already exists', async ({ page }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Alpha');
    await page.locator('#url').fill('https://alpha-unique.example.com');
    await page.getByRole('button', { name: 'Create' }).click();
    await expect(page.getByText('Saved successfully.')).toBeVisible();

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Beta');
    await page.locator('#url').fill('https://beta-unique.example.com');
    await page.getByRole('button', { name: 'Create' }).click();
    await expect(page.getByText('Saved successfully.')).toBeVisible();

    await page
      .getByRole('row', { name: /Beta/ })
      .getByRole('link', { name: 'Edit' })
      .click();
    await page.locator('#url').fill('https://alpha-unique.example.com');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page.locator('.alert-danger')).toContainText('A monitor with this URL already exists');
    await expect(page.locator('#url')).toHaveValue('https://alpha-unique.example.com');
  });

  test('rejects bulk create when a URL already exists', async ({ page, request }) => {
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (name, url) VALUES ('Existing', 'https://bulk-dup.example.com')`,
    });

    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill('https://bulk-new.example.com\nhttps://bulk-dup.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors/bulk');
    await expect(page.locator('.alert-danger')).toContainText('A monitor with this URL already exists');

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('1');
  });
});
