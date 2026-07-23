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

async function expectAdminListPagination(
  page: Page,
  path: string,
  label: string,
  tableSelector: string,
  page2RowCount: number,
  totalCountText?: string,
) {
  await page.goto(path);
  await expect(page.locator(`${tableSelector} tbody tr`)).toHaveCount(100);
  if (totalCountText) {
    await expect(page.getByText(totalCountText, { exact: true })).toBeVisible();
  }

  const pagination = page.getByLabel(`${label} pagination`);
  await expect(pagination.getByRole('link', { name: '2', exact: true })).toBeVisible();
  await expect(pagination.locator('.page-item.active .page-link')).toHaveText('1');

  await pagination.getByRole('link', { name: '2', exact: true }).click();
  await expect(page).toHaveURL(`${path}?page=2`);
  await expect(page.locator(`${tableSelector} tbody tr`)).toHaveCount(page2RowCount);
  if (totalCountText) {
    await expect(page.getByText(totalCountText, { exact: true })).toBeVisible();
  }
}

test.describe('Admin list pagination', () => {
  test('paginates users', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO users (username, password_hash, created_at, updated_at)
              SELECT 'paguser' || n, 'hash', NOW(), NOW()
              FROM generate_series(1, 101) AS n`,
    });

    // login creates one admin user, then 101 more are inserted
    await expectAdminListPagination(page, '/admin/users', 'Users', '.users-table', 2, '102 users.');
  });

  test('paginates monitors', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              SELECT 'https://pag' || n || '.example.com', 'Pag ' || n, NOW(), NOW()
              FROM generate_series(1, 101) AS n`,
    });

    await expectAdminListPagination(page, '/admin/monitors', 'Monitors', '.monitors-table', 1, '101 monitors.');
  });

  test('paginates heartbeats', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://hb-pagination.example.com', 'HB Pagination', NOW(), NOW())`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT m.id, NOW() - (n || ' seconds')::interval, true
              FROM monitor_urls m
              CROSS JOIN generate_series(1, 101) AS n
              WHERE m.url = 'https://hb-pagination.example.com'`,
    });

    await expectAdminListPagination(page, '/admin/heartbeats', 'Heartbeats', '.heartbeats-table', 1);
  });

  test('paginates incidents', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://inc-pagination.example.com', 'Inc Pagination', NOW(), NOW())`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, resolved_at, error_message)
              SELECT m.id, NOW() - (n || ' minutes')::interval, NOW() - ((n - 1) || ' minutes')::interval, 'incident ' || n
              FROM monitor_urls m
              CROSS JOIN generate_series(1, 101) AS n
              WHERE m.url = 'https://inc-pagination.example.com'`,
    });

    await expectAdminListPagination(page, '/admin/incidents', 'Incidents', '.incidents-table', 1, '101 incidents.');
  });

  test('paginates application logs', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-applog');
    await apiCall(request, 'seed-applog', { kind: 'events', count: 101 });

    await expectAdminListPagination(page, '/admin/logs', 'Application Logs', '.log-table', 1);
    await expect(page.locator('.log-table tbody').getByText('pagination event 1')).toBeVisible();
  });

  test('paginates monitor requests', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-applog');
    await apiCall(request, 'seed-applog', { kind: 'requests', count: 101 });

    await expectAdminListPagination(page, '/admin/requests', 'Monitor Requests', '.requests-table', 1);
    await expect(page.locator('.requests-table tbody').getByText('Pagination Test')).toBeVisible();
  });

  test('paginates application errors', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-applog');
    await apiCall(request, 'seed-applog', { kind: 'errors', count: 101 });

    await expectAdminListPagination(page, '/admin/errors', 'Application Errors', '.log-table', 1);
    await expect(page.locator('.log-table tbody').getByText('pagination error 1')).toBeVisible();
  });
});
