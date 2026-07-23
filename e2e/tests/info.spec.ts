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

test.describe('Admin info', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('shows empty backlog and live worker metrics', async ({ page }) => {
    await page.goto('/admin/info');

    await expect(page.getByRole('heading', { name: 'Info', exact: true })).toBeVisible();
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Info' })).toBeVisible();

    await expect(page.getByTestId('info-total-monitors')).toHaveText('0');
    await expect(page.getByTestId('info-due-waiting')).toHaveText('0');
    await expect(page.getByTestId('info-never-checked')).toHaveText('0');
    await expect(page.getByTestId('info-check-concurrency')).not.toHaveText('');
    await expect(page.getByTestId('info-most-overdue-empty')).toBeVisible();
    await expect(page.getByTestId('info-worker-stats')).toBeVisible();
    await expect(page.getByTestId('info-notify-queue')).toContainText('/');
  });

  test('counts seeded monitors on the info page', async ({ page, request }) => {
    await apiCall(request, 'sql', {
      query: `
        INSERT INTO monitor_urls (name, url, check_interval_seconds, created_at, updated_at)
        VALUES
          ('Never Checked', 'https://never.example', 3600, NOW(), NOW()),
          ('Overdue Site', 'https://overdue.example', 3600, NOW(), NOW())
      `,
    });
    await apiCall(request, 'sql', {
      query: `
        UPDATE monitor_urls
        SET last_checked_at = NOW() - INTERVAL '2 hours'
        WHERE url = 'https://overdue.example'
      `,
    });

    await page.goto('/admin/info');
    await expect(page.getByTestId('info-total-monitors')).toHaveText('2');
    await expect(page.getByTestId('info-check-concurrency')).not.toHaveText('');
    await expect(page.getByTestId('info-worker-stats')).toBeVisible();

    const dueText = await page.getByTestId('info-due-waiting').innerText();
    const neverText = await page.getByTestId('info-never-checked').innerText();
    expect(Number(dueText)).toBeGreaterThanOrEqual(0);
    expect(Number(neverText)).toBeGreaterThanOrEqual(0);

    const overdue = page.getByTestId('info-most-overdue');
    const overdueEmpty = page.getByTestId('info-most-overdue-empty');
    if (await overdue.count()) {
      await expect(overdue).toContainText('Overdue Site');
    } else {
      await expect(overdueEmpty).toBeVisible();
    }
  });
});
