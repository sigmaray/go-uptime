import { test, expect, APIRequestContext, Page } from '@playwright/test';

async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown> = {}) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

async function login(page: Page, request: APIRequestContext) {
  await apiCall(request, 'clear-table', { table: 'users' });
  await apiCall(request, 'create-user', { username: 'admin', password: 'password123' });

  await page.goto('/login');
  await page.locator('#username').fill('admin');
  await page.locator('#password').fill('password123');
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page).toHaveURL('/admin/', { timeout: 15000 });
}

test('pauses and resumes a monitor without deleting it', async ({ page, request }) => {
  await login(page, request);
  await apiCall(request, 'clear-table', { table: 'stat_minutely' });
  await apiCall(request, 'clear-table', { table: 'incidents' });
  await apiCall(request, 'clear-table', { table: 'monitor_checks' });
  await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  await apiCall(request, 'sql', {
    query: `INSERT INTO monitor_urls (name, url, active, created_at, updated_at)
            VALUES ('Pause flow', 'https://pause-flow.example.com', true, NOW(), NOW())`,
  });

  await page.goto('/admin/monitors/1');
  await expect(page.getByRole('button', { name: 'Pause' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Delete' })).toBeVisible();

  await page.goto('/admin/monitors');
  const row = page.getByRole('row', { name: /https:\/\/pause-flow\.example\.com/ });
  await expect(row.getByText('Unknown')).toBeVisible();
  const activeActionTops = await Promise.all([
    row.getByRole('link', { name: 'Edit' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Pause' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Delete' }).evaluate((element) => element.getBoundingClientRect().top),
  ]);
  expect(Math.max(...activeActionTops) - Math.min(...activeActionTops)).toBeLessThan(1);
  await page.setViewportSize({ width: 700, height: 900 });
  const narrowActionTops = await Promise.all([
    row.getByRole('link', { name: 'Edit' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Pause' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Delete' }).evaluate((element) => element.getBoundingClientRect().top),
  ]);
  expect(new Set(narrowActionTops.map(Math.round)).size).toBe(3);
  await page.setViewportSize({ width: 1280, height: 720 });
  await row.getByRole('button', { name: 'Pause' }).click();

  await expect(page.getByText('Monitor paused.')).toBeVisible();
  await expect(row.getByText('Paused')).toBeVisible();
  await expect(row.getByRole('button', { name: 'Resume' })).toBeVisible();
  await page.goto('/admin/monitors/1');
  await expect(page.getByRole('button', { name: 'Resume' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Delete' })).toBeVisible();
  await page.goto('/admin/monitors');
  let state = await apiCall(request, 'sql', {
    query: `SELECT active::text FROM monitor_urls WHERE url = 'https://pause-flow.example.com'`,
  });
  expect(state.rows[0][0]).toBe('false');

  await row.getByRole('button', { name: 'Resume' }).click();
  await expect(page.getByText('Monitor resumed.')).toBeVisible();
  await expect(row.getByRole('button', { name: 'Pause' })).toBeVisible();
  const resumedActionTops = await Promise.all([
    row.getByRole('link', { name: 'Edit' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Pause' }).evaluate((element) => element.getBoundingClientRect().top),
    row.getByRole('button', { name: 'Delete' }).evaluate((element) => element.getBoundingClientRect().top),
  ]);
  expect(Math.max(...resumedActionTops) - Math.min(...resumedActionTops)).toBeLessThan(1);
  state = await apiCall(request, 'sql', {
    query: `SELECT active::text FROM monitor_urls WHERE url = 'https://pause-flow.example.com'`,
  });
  expect(state.rows[0][0]).toBe('true');

  await page.goto('/admin/tools/');
  await page.getByRole('button', { name: 'Pause all monitors' }).click();
  await expect(page.getByText('Paused all monitors (1 changed).')).toBeVisible();
  state = await apiCall(request, 'sql', {
    query: `SELECT active::text FROM monitor_urls WHERE url = 'https://pause-flow.example.com'`,
  });
  expect(state.rows[0][0]).toBe('false');

  await page.getByRole('button', { name: 'Resume all monitors' }).click();
  await expect(page.getByText('Resumed all monitors (1 changed).')).toBeVisible();
  state = await apiCall(request, 'sql', {
    query: `SELECT active::text FROM monitor_urls WHERE url = 'https://pause-flow.example.com'`,
  });
  expect(state.rows[0][0]).toBe('true');
});
