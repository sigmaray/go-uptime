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

async function monitorIDs(page: Page): Promise<string[]> {
  return page.locator('.monitors-table tbody tr td:first-child a').allTextContents();
}

async function monitorURLs(page: Page): Promise<string[]> {
  return page.locator('.monitors-table tbody tr td:nth-child(2) a').allTextContents();
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

    await expect(page.getByRole('link', { name: 'Sort by ID ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by ID descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by URL ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by URL descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Last Check ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Last Check descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Error ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Error descending' })).toBeVisible();

    await expect(page.getByRole('link', { name: /Sort by Name/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Uptime/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Last 30 min/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Edit/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Sort by Delete/i })).toHaveCount(0);
  });

  test('sorts monitors by id in both directions', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at) VALUES
              ('https://charlie.example.com', 'Charlie', NOW() - INTERVAL '3 minutes', NOW()),
              ('https://alpha.example.com', 'Alpha', NOW() - INTERVAL '2 minutes', NOW()),
              ('https://bravo.example.com', 'Bravo', NOW() - INTERVAL '1 minute', NOW())`,
    });
    const idsResult = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls ORDER BY id ASC`,
    });
    const idsAsc = (idsResult.rows as string[][])
      .map((row) => row[0])
      .sort((a, b) => Number(a) - Number(b));
    const idsDesc = [...idsAsc].reverse();

    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Sort by ID ascending' }).click();
    await expect(page).toHaveURL(/sort=ID/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(await monitorIDs(page)).toEqual(idsAsc);
    await expect(page.getByRole('link', { name: 'Sort by ID ascending' })).toHaveClass(/table-sort__link--active/);

    await page.getByRole('link', { name: 'Sort by ID descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(await monitorIDs(page)).toEqual(idsDesc);
    await expect(page.getByRole('link', { name: 'Sort by ID descending' })).toHaveClass(/table-sort__link--active/);
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
    await expect(await monitorURLs(page)).toEqual([
      'https://alpha.example.com',
      'https://mike.example.com',
      'https://zulu.example.com',
    ]);

    await page.getByRole('link', { name: 'Sort by Status ascending' }).click();
    await expect(await monitorURLs(page)).toEqual([
      'https://zulu.example.com',
      'https://alpha.example.com',
      'https://mike.example.com',
    ]);

    await page.getByRole('link', { name: 'Sort by Last Check ascending' }).click();
    await expect(await monitorURLs(page)).toEqual([
      'https://zulu.example.com',
      'https://mike.example.com',
      'https://alpha.example.com',
    ]);

    await page.getByRole('link', { name: 'Sort by Error ascending' }).click();
    await expect(await monitorURLs(page)).toEqual([
      'https://alpha.example.com',
      'https://mike.example.com',
      'https://zulu.example.com',
    ]);
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
    const idsResult = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls ORDER BY id ASC`,
    });
    const idsAsc = (idsResult.rows as string[][])
      .map((row) => row[0])
      .sort((a, b) => Number(a) - Number(b));

    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Sort by ID ascending' }).click();
    await expect(page).toHaveURL(/sort=ID/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(100);
    await expect(await monitorIDs(page)).toEqual(idsAsc.slice(0, 100));

    const pagination = page.getByLabel('Monitors pagination');
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /sort=ID/,
    );
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /order=asc/,
    );

    await pagination.getByRole('link', { name: '2', exact: true }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page).toHaveURL(/sort=ID/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(1);
    await expect(await monitorIDs(page)).toEqual([idsAsc[100]]);

    await page.getByRole('link', { name: 'Sort by ID descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(page).not.toHaveURL(/page=/);
    await expect(await monitorIDs(page)).toEqual([...idsAsc].reverse().slice(0, 100));
  });

  test('id links to monitor detail page', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://id-link.example.com', 'ID Link', NOW(), NOW())`,
    });
    const idsResult = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = 'https://id-link.example.com'`,
    });
    const id = (idsResult.rows as string[][])[0][0];

    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: id, exact: true }).click();
    await expect(page).toHaveURL(`/admin/monitors/${id}`);
    await expect(page.getByRole('heading', { name: 'ID Link' })).toBeVisible();
  });
});
