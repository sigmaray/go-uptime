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

async function clearMonitors(request: APIRequestContext) {
  await apiCall(request, 'clear-table', { table: 'incidents' });
  await apiCall(request, 'clear-table', { table: 'monitor_checks' });
  await apiCall(request, 'clear-table', { table: 'monitor_urls' });
}

test.describe('Verify before create', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
  });

  test('shows verify checkbox on single and bulk forms', async ({ page }) => {
    await page.goto('/admin/monitors/new');
    await expect(page.locator('#verify_before_create')).toBeVisible();
    await expect(page.locator('#verify_before_create')).not.toBeChecked();

    await page.goto('/admin/monitors/bulk/new');
    await expect(page.locator('#verify_before_create')).toBeVisible();
    await expect(page.locator('#verify_before_create')).not.toBeChecked();
  });

  test('rejects unavailable URL when verify is checked on single form', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('http://127.0.0.1:1');
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.locator('.alert-danger')).toContainText('unavailable');
    await expect(page.locator('#verify_before_create')).toBeChecked();
    await expect(page.locator('#url')).toHaveValue('http://127.0.0.1:1');

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('0');
  });

  test('creates reachable URL when verify is checked on single form', async ({ page }) => {
    // Probe runs inside the app container, which listens on 8080 (not host-mapped 18081).
    const reachableURL = 'http://127.0.0.1:8080/health';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Local Health');
    await page.locator('#url').fill(reachableURL);
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText(reachableURL)).toBeVisible();
  });

  test('rejects bulk create when verify is checked and any URL is unavailable', async ({ page, request }) => {
    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill('http://127.0.0.1:8080/health\nhttp://127.0.0.1:1');
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors/bulk');
    await expect(page.locator('.alert-danger')).toContainText('unavailable');
    await expect(page.locator('#verify_before_create')).toBeChecked();

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('0');
  });

  test('creates bulk monitors when verify is checked and all URLs are reachable', async ({ page }) => {
    const urls = ['http://127.0.0.1:8080/health', 'http://127.0.0.1:8080/login'];

    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill(urls.join('\n'));
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    for (const url of urls) {
      await expect(page.getByText(url)).toBeVisible();
    }
  });
});
