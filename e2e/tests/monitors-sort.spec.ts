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
  await expect(page.getByText('Invalid username or password')).toHaveCount(0);
  await expect(page).toHaveURL('/admin/', { timeout: 15000 });
}

async function monitorNames(page: Page): Promise<string[]> {
  return page.locator('.monitors-table tbody tr td:first-child a').allTextContents();
}

test.describe('Monitors list sorting', () => {
  test('shows sort links on sortable columns only', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://sort-links.example.com', 'Sort Links', NOW(), NOW())`,
    });

    await page.goto('/admin/monitors');

    await expect(page.getByRole('link', { name: 'Sort by Name ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Name descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by URL ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by URL descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Last Check ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Last Check descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Error ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Error descending' })).toBeVisible();

    await expect(page.getByRole('link', { name: /Sort by Uptime/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Last 30 min/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Edit/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Delete/i })).toHaveCount(0);
  });

  test('sorts monitors by name in both directions', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at) VALUES
              ('https://charlie.example.com', 'Charlie', NOW() - INTERVAL '3 minutes', NOW()),
              ('https://alpha.example.com', 'Alpha', NOW() - INTERVAL '2 minutes', NOW()),
              ('https://bravo.example.com', 'Bravo', NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Sort by Name ascending' }).click();
    await expect(page).toHaveURL(/sort=Name/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(await monitorNames(page)).toEqual(['Alpha', 'Bravo', 'Charlie']);
    await expect(page.getByRole('link', { name: 'Sort by Name ascending' })).toHaveClass(/table-sort__link--active/);

    await page.getByRole('link', { name: 'Sort by Name descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(await monitorNames(page)).toEqual(['Charlie', 'Bravo', 'Alpha']);
    await expect(page.getByRole('link', { name: 'Sort by Name descending' })).toHaveClass(/table-sort__link--active/);
  });

  test('sorts monitors by url, status, last check and error', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'stat_minutely' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, last_error, created_at, updated_at) VALUES
              ('https://zulu.example.com', 'Zulu', false, NOW() - INTERVAL '3 hours', 'zzz timeout', NOW() - INTERVAL '3 minutes', NOW()),
              ('https://alpha.example.com', 'Alpha', true, NOW() - INTERVAL '1 hour', 'aaa refused', NOW() - INTERVAL '2 minutes', NOW()),
              ('https://mike.example.com', 'Mike', true, NOW() - INTERVAL '2 hours', 'mmm reset', NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');

    await page.getByRole('link', { name: 'Sort by URL ascending' }).click();
    await expect(await monitorNames(page)).toEqual(['Alpha', 'Mike', 'Zulu']);

    await page.getByRole('link', { name: 'Sort by Status ascending' }).click();
    await expect(await monitorNames(page)).toEqual(['Zulu', 'Alpha', 'Mike']);

    await page.getByRole('link', { name: 'Sort by Last Check ascending' }).click();
    await expect(await monitorNames(page)).toEqual(['Zulu', 'Mike', 'Alpha']);

    await page.getByRole('link', { name: 'Sort by Error ascending' }).click();
    await expect(await monitorNames(page)).toEqual(['Alpha', 'Mike', 'Zulu']);
  });

  test('keeps sort when paginating monitors', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              SELECT 'https://sortpag' || n || '.example.com',
                     'SortPag ' || lpad(n::text, 3, '0'),
                     NOW() - (n || ' seconds')::interval,
                     NOW()
              FROM generate_series(1, 101) AS n`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Sort by Name ascending' }).click();
    await expect(page).toHaveURL(/sort=Name/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(100);
    await expect(await monitorNames(page)).toEqual(
      Array.from({ length: 100 }, (_, i) => `SortPag ${String(i + 1).padStart(3, '0')}`),
    );

    const pagination = page.getByLabel('Monitors pagination');
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /sort=Name/,
    );
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /order=asc/,
    );

    await pagination.getByRole('link', { name: '2', exact: true }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page).toHaveURL(/sort=Name/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(1);
    await expect(await monitorNames(page)).toEqual(['SortPag 101']);

    await page.getByRole('link', { name: 'Sort by Name descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(page).not.toHaveURL(/page=/);
    await expect(await monitorNames(page)).toEqual(
      Array.from({ length: 100 }, (_, i) => `SortPag ${String(101 - i).padStart(3, '0')}`),
    );
  });
});
