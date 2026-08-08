import { test, expect, APIRequestContext, Page } from '@playwright/test';

// E2e-тесты сортировки списка heartbeats: колонки, порядок, сохранение sort при пагинации.

/** POST к dev-only Playwright API; падает тест, если сервер вернул не 2xx. */
async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown> = {}) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

/** Логин admin + проверка успешного входа (timeout 15s — Docker/воркер могут стартовать медленно). */
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

/** Имена мониторов из колонки Monitor (только текст узла, без вложенных ссылок). */
async function heartbeatMonitorNames(page: Page): Promise<string[]> {
  return page.locator('.heartbeats-table tbody tr td:nth-child(2)').evaluateAll((cells) =>
    cells.map((cell) => (cell.childNodes[0]?.textContent ?? '').trim()),
  );
}

/** Статусы Up/Down из badge в колонке Status. */
async function heartbeatStatuses(page: Page): Promise<string[]> {
  return page.locator('.heartbeats-table tbody tr td:nth-child(5) .badge').allTextContents();
}

/** Response time из колонки Response Time. */
async function heartbeatResponseTimes(page: Page): Promise<string[]> {
  return page.locator('.heartbeats-table tbody tr td:nth-child(4)').allTextContents();
}

test.describe('Heartbeats list sorting', () => {
  test('shows sort links on all columns', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://hb-sort-links.example.com', 'HB Sort Links', NOW(), NOW())`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW(), true, 100 FROM monitor_urls WHERE name = 'HB Sort Links'`,
    });

    await page.goto('/admin/heartbeats');

    // Все колонки таблицы heartbeats поддерживают сортировку asc/desc.
    await expect(page.getByRole('link', { name: 'Sort by Monitor ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Monitor descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Checked At ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Checked At descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Response Time ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Response Time descending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status ascending' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sort by Status descending' })).toBeVisible();
  });

  test('sorts heartbeats by monitor in both directions', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at) VALUES
              ('https://charlie-hb.example.com', 'Charlie HB', NOW(), NOW()),
              ('https://alpha-hb.example.com', 'Alpha HB', NOW(), NOW()),
              ('https://bravo-hb.example.com', 'Bravo HB', NOW(), NOW())`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '1 minute', true, 50 FROM monitor_urls WHERE name = 'Charlie HB'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '2 minutes', true, 60 FROM monitor_urls WHERE name = 'Alpha HB'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '3 minutes', true, 70 FROM monitor_urls WHERE name = 'Bravo HB'`,
    });

    await page.goto('/admin/heartbeats');
    await page.getByRole('link', { name: 'Sort by Monitor ascending' }).click();
    await expect(page).toHaveURL(/sort=MonitorURL/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(await heartbeatMonitorNames(page)).toEqual(['Alpha HB', 'Bravo HB', 'Charlie HB']);
    await expect(page.getByRole('link', { name: 'Sort by Monitor ascending' })).toHaveClass(/table-sort__link--active/);

    await page.getByRole('link', { name: 'Sort by Monitor descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(await heartbeatMonitorNames(page)).toEqual(['Charlie HB', 'Bravo HB', 'Alpha HB']);
    await expect(page.getByRole('link', { name: 'Sort by Monitor descending' })).toHaveClass(/table-sort__link--active/);
  });

  test('sorts heartbeats by checked at, response time and status', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at) VALUES
              ('https://slow.example.com', 'Slow', NOW(), NOW()),
              ('https://fast.example.com', 'Fast', NOW(), NOW()),
              ('https://down.example.com', 'Down', NOW(), NOW())`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '3 hours', true, 300 FROM monitor_urls WHERE name = 'Slow'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '1 hour', true, 50 FROM monitor_urls WHERE name = 'Fast'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW() - INTERVAL '2 hours', false, 200 FROM monitor_urls WHERE name = 'Down'`,
    });

    await page.goto('/admin/heartbeats');

    await page.getByRole('link', { name: 'Sort by Checked At ascending' }).click();
    await expect(await heartbeatMonitorNames(page)).toEqual(['Slow', 'Down', 'Fast']);

    await page.getByRole('link', { name: 'Sort by Response Time ascending' }).click();
    await expect(await heartbeatResponseTimes(page)).toEqual(['50 ms', '200 ms', '300 ms']);
    await expect(await heartbeatMonitorNames(page)).toEqual(['Fast', 'Down', 'Slow']);

    await page.getByRole('link', { name: 'Sort by Status ascending' }).click();
    await expect(await heartbeatStatuses(page)).toEqual(['Down', 'Up', 'Up']);
    await expect(await heartbeatMonitorNames(page)).toEqual(['Down', 'Slow', 'Fast']);
  });

  test('keeps sort when paginating heartbeats', async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at)
              VALUES ('https://hb-pagination-sort.example.com', 'HB Pagination Sort', NOW(), NOW())`,
    });
    // 101 heartbeat с response_time_ms = n — сортировка по ResponseTimeMs детерминирована.
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT m.id, NOW() - (n || ' seconds')::interval, true, n
              FROM monitor_urls m
              CROSS JOIN generate_series(1, 101) AS n
              WHERE m.url = 'https://hb-pagination-sort.example.com'`,
    });

    await page.goto('/admin/heartbeats');
    await page.getByRole('link', { name: 'Sort by Response Time ascending' }).click();
    await expect(page).toHaveURL(/sort=ResponseTimeMs/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.heartbeats-table tbody tr')).toHaveCount(100);
    await expect(await heartbeatResponseTimes(page)).toEqual(
      Array.from({ length: 100 }, (_, i) => `${i + 1} ms`),
    );

    const pagination = page.getByLabel('Heartbeats pagination');
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /sort=ResponseTimeMs/,
    );
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /order=asc/,
    );

    await pagination.getByRole('link', { name: '2', exact: true }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page).toHaveURL(/sort=ResponseTimeMs/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.locator('.heartbeats-table tbody tr')).toHaveCount(1);
    await expect(await heartbeatResponseTimes(page)).toEqual(['101 ms']);

    await page.getByRole('link', { name: 'Sort by Response Time descending' }).click();
    await expect(page).toHaveURL(/order=desc/);
    await expect(page).not.toHaveURL(/page=/);
    await expect(await heartbeatResponseTimes(page)).toEqual(
      Array.from({ length: 100 }, (_, i) => `${101 - i} ms`),
    );
  });
});
