import { test, expect, APIRequestContext, Page } from '@playwright/test';

// E2e-тесты фильтра и поиска списка мониторов: status All/Down/Up, URL search, комбинации с sort и пагинацией.

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

/** Очистка мониторов и связанных таблиц (включая stat_minutely для uptime-колонок). */
async function clearMonitors(request: APIRequestContext) {
  await apiCall(request, 'clear-table', { table: 'stat_minutely' });
  await apiCall(request, 'clear-table', { table: 'incidents' });
  await apiCall(request, 'clear-table', { table: 'monitor_checks' });
  await apiCall(request, 'clear-table', { table: 'monitor_urls' });
}

/** URL из второй колонки таблицы мониторов. */
async function monitorURLs(page: Page): Promise<string[]> {
  return page.locator('.monitors-table tbody tr td:nth-child(2) a').allTextContents();
}

/** Тексты badge статуса (Up/Down) из строк таблицы. */
async function monitorStatuses(page: Page): Promise<string[]> {
  return page.locator('.monitors-table tbody tr td .badge').allTextContents();
}

test.describe('Monitors list filter and search', () => {
  test('filters monitors by All / Down / Up status', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at) VALUES
              ('https://up.example.com', 'Up Host', true, NOW(), NOW() - INTERVAL '3 minutes', NOW()),
              ('https://down.example.com', 'Down Host', false, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
              ('https://unknown.example.com', 'Unknown Host', NULL, NULL, NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await expect(page.getByText('3 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual([
      'https://unknown.example.com',
      'https://down.example.com',
      'https://up.example.com',
    ]);

    const filter = page.getByRole('search', { name: 'Filter monitors' });
    await expect(filter.getByRole('link', { name: 'All' })).toHaveClass(/active/);

    // Фильтр Down — только упавший монитор.
    await filter.getByRole('link', { name: 'Down' }).click();
    await expect(page).toHaveURL(/status=down/);
    await expect(page.getByText('1 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual(['https://down.example.com']);
    await expect(await monitorStatuses(page)).toEqual(['Down']);
    await expect(filter.getByRole('link', { name: 'Down' })).toHaveClass(/active/);

    // Фильтр Up.
    await filter.getByRole('link', { name: 'Up' }).click();
    await expect(page).toHaveURL(/status=up/);
    await expect(page.getByText('1 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual(['https://up.example.com']);
    await expect(await monitorStatuses(page)).toEqual(['Up']);

    // All — снова три монитора, query param status убирается.
    await filter.getByRole('link', { name: 'All' }).click();
    await expect(page).not.toHaveURL(/status=/);
    await expect(page.getByText('3 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual([
      'https://unknown.example.com',
      'https://down.example.com',
      'https://up.example.com',
    ]);
  });

  test('searches monitors by URL fragment', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, created_at, updated_at) VALUES
              ('https://api.example.com/health', 'API', NOW() - INTERVAL '3 minutes', NOW()),
              ('https://www.example.com/', 'WWW', NOW() - INTERVAL '2 minutes', NOW()),
              ('https://other.test/status', 'Other', NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.locator('#monitors-url-search').fill('example.com');
    await page.getByRole('button', { name: 'Search' }).click();

    await expect(page).toHaveURL(/q=example\.com/);
    await expect(page.getByText('2 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual([
      'https://www.example.com/',
      'https://api.example.com/health',
    ]);
    await expect(page.locator('#monitors-url-search')).toHaveValue('example.com');

    // Поиск регистронезависимый (OTHER.TEST находит other.test).
    await page.locator('#monitors-url-search').fill('OTHER.TEST');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(/q=OTHER\.TEST/);
    await expect(page.getByText('1 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual(['https://other.test/status']);
  });

  test('combines status filter with URL search', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at) VALUES
              ('https://api.example.com/up', 'API Up', true, NOW(), NOW() - INTERVAL '4 minutes', NOW()),
              ('https://api.example.com/down', 'API Down', false, NOW(), NOW() - INTERVAL '3 minutes', NOW()),
              ('https://web.example.com/up', 'Web Up', true, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
              ('https://web.example.com/down', 'Web Down', false, NOW(), NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.locator('#monitors-url-search').fill('api.example');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(await monitorURLs(page)).toEqual([
      'https://api.example.com/down',
      'https://api.example.com/up',
    ]);

    // status=down + q=api.example → один результат.
    await page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' }).click();
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/q=api\.example/);
    await expect(page.getByText('1 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorURLs(page)).toEqual(['https://api.example.com/down']);
    await expect(page.locator('#monitors-url-search')).toHaveValue('api.example');
  });

  test('keeps filters when paginating monitors', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    // 101 down + 1 up с общим фрагментом downpag в URL.
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at)
              SELECT
                'https://downpag' || n || '.example.com/path',
                'DownPag ' || lpad(n::text, 3, '0'),
                false,
                NOW(),
                NOW() - (n || ' seconds')::interval,
                NOW()
              FROM generate_series(1, 101) AS n`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at)
              VALUES ('https://uppag.example.com/path', 'UpPag', true, NOW(), NOW(), NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.locator('#monitors-url-search').fill('downpag');
    await page.getByRole('button', { name: 'Search' }).click();
    await page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' }).click();

    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/q=downpag/);
    await expect(page.getByText('101 monitors.', { exact: true })).toBeVisible();
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(100);
    await expect(await monitorStatuses(page)).toEqual(Array(100).fill('Down'));

    const pagination = page.getByLabel('Monitors pagination');
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /status=down/,
    );
    await expect(pagination.getByRole('link', { name: '2', exact: true })).toHaveAttribute(
      'href',
      /q=downpag/,
    );

    await pagination.getByRole('link', { name: '2', exact: true }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/q=downpag/);
    await expect(page.locator('.monitors-table tbody tr')).toHaveCount(1);
    await expect(page.getByText('101 monitors.', { exact: true })).toBeVisible();
    await expect(await monitorStatuses(page)).toEqual(['Down']);
  });

  test('keeps filters when sorting monitors', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at) VALUES
              ('https://bravo.example.com', 'Bravo', false, NOW(), NOW() - INTERVAL '3 minutes', NOW()),
              ('https://alpha.example.com', 'Alpha', false, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
              ('https://charlie.example.com', 'Charlie', true, NOW(), NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' }).click();
    await page.getByRole('link', { name: 'Sort by URL ascending' }).click();

    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/sort=URL/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(await monitorURLs(page)).toEqual([
      'https://alpha.example.com',
      'https://bravo.example.com',
    ]);

    // Ссылка desc должна сохранять status=down в query.
    await expect(page.getByRole('link', { name: 'Sort by URL descending' })).toHaveAttribute(
      'href',
      /status=down/,
    );
  });

  test('returns to filtered list after editing a monitor', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at) VALUES
              ('https://bravo.example.com', 'Bravo', false, NOW(), NOW() - INTERVAL '3 minutes', NOW()),
              ('https://alpha.example.com', 'Alpha', false, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
              ('https://charlie.example.com', 'Charlie', true, NOW(), NOW() - INTERVAL '1 minute', NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' }).click();
    await page.getByRole('link', { name: 'Sort by URL ascending' }).click();
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/sort=URL/);
    await expect(page).toHaveURL(/order=asc/);

    // Edit передаёт return_to с текущими filter/sort query params.
    await page.getByRole('row', { name: /alpha\.example\.com/ }).getByRole('link', { name: 'Edit' }).click();
    await expect(page).toHaveURL(/return_to=/);
    await page.locator('#name').fill('Alpha Renamed');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page).toHaveURL(/\/admin\/monitors\?/);
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/sort=URL/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });

  test('returns to filtered list after deleting a monitor', async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_urls (url, name, is_up, last_checked_at, created_at, updated_at) VALUES
              ('https://keep.example.com', 'Keep', false, NOW(), NOW() - INTERVAL '2 minutes', NOW()),
              ('https://remove.example.com', 'Remove', false, NOW(), NOW() - INTERVAL '1 minute', NOW()),
              ('https://up.example.com', 'Up', true, NOW(), NOW(), NOW())`,
    });

    await page.goto('/admin/monitors');
    await page.getByRole('search', { name: 'Filter monitors' }).getByRole('link', { name: 'Down' }).click();
    await page.getByRole('link', { name: 'Sort by URL ascending' }).click();
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/sort=URL/);

    // confirm() на Delete — page.once принимает диалог один раз.
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('row', { name: /remove\.example\.com/ }).getByRole('button', { name: 'Delete' }).click();

    await expect(page).toHaveURL(/\/admin\/monitors\?/);
    await expect(page).toHaveURL(/status=down/);
    await expect(page).toHaveURL(/sort=URL/);
    await expect(page).toHaveURL(/order=asc/);
    await expect(page.getByText('Deleted successfully.')).toBeVisible();
    await expect(await monitorURLs(page)).toEqual(['https://keep.example.com']);
  });
});
