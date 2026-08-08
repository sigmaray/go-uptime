import { test, expect, APIRequestContext } from '@playwright/test';

// E2e-тесты CRUD пользователей, мониторов, heartbeats, incidents, logs, settings и dev tools.
// Большой набор сценариев UI + seed данных через /api/playwright (SQL, clear-table).

/** POST к dev-only Playwright API; падает тест, если сервер вернул не 2xx. */
async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown>) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

/** Логин admin + проверка успешного входа (timeout 15s — Docker/воркер могут стартовать медленно). */
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

test.describe('Users CRUD', () => {
  test('creates, edits and deletes a user', async ({ page, request }) => {
    await login(page, request);

    // Create user через UI.
    await page.goto('/admin/users');
    await page.getByRole('link', { name: 'Create User' }).click();
    await page.locator('#username').fill('newuser');
    await page.locator('#password').fill('secret123');
    await page.locator('#confirm_password').fill('secret123');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/users');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('newuser')).toBeVisible();

    // Edit: переименование.
    await page.getByRole('row', { name: /newuser/ }).getByRole('link', { name: 'Edit' }).click();
    await page.locator('#username').fill('renamed');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page).toHaveURL('/admin/users');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('renamed')).toBeVisible();

    // Delete с подтверждением confirm().
    page.on('dialog', (dialog) => dialog.accept());
    await page.getByRole('row', { name: /renamed/ }).getByRole('button', { name: 'Delete' }).click();
    await expect(page).toHaveURL('/admin/users');
    await expect(page.getByText('Deleted successfully.')).toBeVisible();
    await expect(page.getByText('renamed')).not.toBeVisible();

    // Flash-сообщение не «перетекает» на другую страницу после redirect.
    await page.goto('/admin/settings');
    await expect(page.getByText('Deleted successfully.')).toHaveCount(0);
  });
});

test.describe('Monitors', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('creates and lists a monitor URL', async ({ page }) => {
    await page.goto('/admin/monitors');
    await page.getByRole('link', { name: 'Add URL' }).click();
    await page.locator('#name').fill('Example');
    await page.locator('#url').fill('https://example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('https://example.com')).toBeVisible();
  });

  test('deletes a monitor URL permanently', async ({ page, request }) => {
    const monitorName = 'Delete Me';
    const monitorUrl = 'https://delete-me.example.com';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill(monitorName);
    await page.locator('#url').fill(monitorUrl);
    await page.getByRole('button', { name: 'Create' }).click();
    await expect(page.getByText(monitorUrl)).toBeVisible();

    // Seed связанных incidents и monitor_checks — delete должен каскадно очистить.
    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, error_message)
              SELECT id, NOW(), 'related incident' FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT id, NOW(), true FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });

    page.on('dialog', (dialog) => dialog.accept());
    await page.getByRole('row', { name: new RegExp(monitorUrl) }).getByRole('button', { name: 'Delete' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Deleted successfully.')).toBeVisible();
    await expect(page.getByText(monitorUrl)).not.toBeVisible();

    const monitors = await apiCall(request, 'sql', {
      query: `SELECT id FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });
    expect(monitors.rows ?? []).toEqual([]);

    const checks = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_checks`,
    });
    expect(checks.rows).toEqual([['0']]);

    const incidents = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM incidents`,
    });
    expect(incidents.rows).toEqual([['0']]);
  });

  test('shows incidents on monitor detail page', async ({ page, request }) => {
    const monitorName = 'Incidents Test';
    const monitorUrl = 'https://incidents.example.com';
    const openError = 'open incident error';
    const resolvedError = 'resolved incident error';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill(monitorName);
    await page.locator('#url').fill(monitorUrl);
    await page.getByRole('button', { name: 'Create' }).click();

    // Open incident (resolved_at NULL) и resolved incident с длительностью 10m.
    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, resolved_at, error_message)
              SELECT id, NOW() - INTERVAL '10 minutes', NULL, '${openError}'
              FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, resolved_at, error_message)
              SELECT id, NOW() - INTERVAL '15 minutes', NOW() - INTERVAL '5 minutes', '${resolvedError}'
              FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });

    await page.goto('/admin/monitors');
    const incidentsMonitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });
    const incidentsMonitorID = (incidentsMonitor.rows as string[][])[0][0];
    await page.getByRole('link', { name: incidentsMonitorID, exact: true }).click();

    await expect(page).toHaveURL(/\/admin\/monitors\/\d+$/);

    const incidentsTable = page.locator('.incidents-table');
    await expect(incidentsTable).toBeVisible();

    const openRow = incidentsTable.locator('tbody tr', { hasText: openError }).first();
    await expect(openRow.locator('span.badge')).toHaveText('Open');
    await expect(openRow).toContainText('ongoing');

    const resolvedRow = incidentsTable.locator('tbody tr', { hasText: resolvedError }).first();
    await expect(resolvedRow.locator('span.badge')).toHaveCount(0);
    await expect(resolvedRow).toContainText('10m0s');
  });

  test('paginates incidents and heartbeats on monitor detail page', async ({ page, request }) => {
    const monitorName = 'Pagination Test';
    const monitorUrl = 'https://pagination.example.com';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill(monitorName);
    await page.locator('#url').fill(monitorUrl);
    await page.getByRole('button', { name: 'Create' }).click();

    // 25 incidents и 25 heartbeats — по 20 на первой странице каждой таблицы.
    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, resolved_at, error_message)
              SELECT m.id, NOW() - (n || ' minutes')::interval, NOW() - ((n - 1) || ' minutes')::interval, 'incident ' || n
              FROM monitor_urls m
              CROSS JOIN generate_series(1, 25) AS n
              WHERE m.url = '${monitorUrl}'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT m.id, NOW() - (n || ' minutes')::interval, true
              FROM monitor_urls m
              CROSS JOIN generate_series(1, 25) AS n
              WHERE m.url = '${monitorUrl}'`,
    });

    await page.goto('/admin/monitors');
    const paginationMonitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = '${monitorUrl}'`,
    });
    const paginationMonitorID = (paginationMonitor.rows as string[][])[0][0];
    await page.getByRole('link', { name: paginationMonitorID, exact: true }).click();

    const incidentsTable = page.locator('.incidents-table');
    const heartbeatsTable = page.locator('.heartbeats-table');

    await expect(incidentsTable.locator('tbody tr')).toHaveCount(20);
    const incidentsPagination = page.getByLabel('Incidents pagination');
    await expect(incidentsPagination.getByRole('link', { name: '2', exact: true })).toBeVisible();
    await expect(incidentsPagination.locator('.page-item.active .page-link')).toHaveText('1');
    await expect(heartbeatsTable.locator('tbody tr')).toHaveCount(20);
    const heartbeatsPagination = page.getByLabel('Heartbeat History pagination');
    await expect(heartbeatsPagination.getByRole('link', { name: '2', exact: true })).toBeVisible();
    await expect(heartbeatsPagination.locator('.page-item.active .page-link')).toHaveText('1');

    // Пагинация incidents не сбрасывает heartbeats_page=1.
    await incidentsPagination.getByRole('link', { name: '2', exact: true }).click();
    await expect(page).toHaveURL(/incidents_page=2/);
    await expect(incidentsTable.locator('tbody tr')).toHaveCount(5);
    await expect(heartbeatsTable.locator('tbody tr')).toHaveCount(20);

    // Next на heartbeats добавляет heartbeats_page=2, incidents_page сохраняется.
    await page.getByLabel('Heartbeat History pagination').getByRole('link', { name: 'Next' }).click();
    await expect(page).toHaveURL(/incidents_page=2/);
    await expect(page).toHaveURL(/heartbeats_page=2/);
    await expect(heartbeatsTable.locator('tbody tr')).toHaveCount(5);
  });

  test('uses URL hostname when name is omitted', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://example.org');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('https://example.org')).toBeVisible();

    const monitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = 'https://example.org'`,
    });
    const monitorID = (monitor.rows as string[][])[0][0];
    await page.getByRole('link', { name: monitorID, exact: true }).click();
    await expect(page.getByRole('heading', { name: 'example.org' })).toBeVisible();
  });

  test('shows uptime history bars', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO stat_minutely (monitor_url_id, bucket_at, up_seconds, total_seconds)
              SELECT id, date_trunc('minute', NOW()), 60, 60
              FROM monitor_urls WHERE url = 'https://example.com'`,
    });

    await page.goto('/admin/monitors');
    await expect(page.locator('.uptime-history__bar--up')).toBeVisible();
  });

  test('shows uptime percentage stats', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://uptime.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    // Монитор старше 24h + stat_minutely 3500/3600 → 97.22%.
    await apiCall(request, 'sql', {
      query: `UPDATE monitor_urls SET created_at = NOW() - INTERVAL '25 hours'
              WHERE url = 'https://uptime.example.com'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO stat_minutely (monitor_url_id, bucket_at, up_seconds, total_seconds)
              SELECT id, date_trunc('minute', NOW()), 3500, 3600
              FROM monitor_urls WHERE url = 'https://uptime.example.com'`,
    });

    await page.goto('/admin/monitors');
    await expect(page.getByText('97.22%').first()).toBeVisible();
  });

  test('hides uptime percentages for monitors younger than reporting period', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://young.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO stat_minutely (monitor_url_id, bucket_at, up_seconds, total_seconds)
              SELECT id, date_trunc('minute', NOW()), 0, 60
              FROM monitor_urls WHERE url = 'https://young.example.com'`,
    });

    await page.goto('/admin/monitors');
    const row = page.getByRole('row', { name: /young\.example\.com/ });
    await expect(row.getByText('0.00%')).toHaveCount(0);
    await expect(row.locator('.uptime-stats__item').filter({ hasText: '24h' }).getByText('—')).toBeVisible();
  });

  test('shows no-data bars before monitor creation', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://recent.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    // stat_minutely в текущей минуте, но до created_at монитора — bar nodata + up.
    await apiCall(request, 'sql', {
      query: `INSERT INTO stat_minutely (monitor_url_id, bucket_at, up_seconds, total_seconds)
              SELECT id, date_trunc('minute', NOW()), 60, 60
              FROM monitor_urls WHERE url = 'https://recent.example.com'`,
    });

    await page.goto('/admin/monitors');
    await expect(page.locator('.uptime-history__bar--nodata').first()).toBeVisible();
    await expect(page.locator('.uptime-history__bar--up')).toBeVisible();
  });

  test('opens monitor detail with heartbeat history', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Detail Test');
    await page.locator('#url').fill('https://detail.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT id, NOW() - INTERVAL '1 minute', true FROM monitor_urls WHERE url = 'https://detail.example.com'`,
    });
    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT id, NOW(), false FROM monitor_urls WHERE url = 'https://detail.example.com'`,
    });

    await page.goto('/admin/monitors');
    const detailMonitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = 'https://detail.example.com'`,
    });
    const detailMonitorID = (detailMonitor.rows as string[][])[0][0];
    await page.getByRole('link', { name: detailMonitorID, exact: true }).click();

    await expect(page).toHaveURL(/\/admin\/monitors\/\d+$/);
    await expect(page.getByRole('heading', { name: 'Detail Test' })).toBeVisible();
    await expect(page.getByText('https://detail.example.com')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Heartbeat History' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Up', exact: true })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Down', exact: true })).toBeVisible();
  });
});

test.describe('Heartbeats', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('lists heartbeats from all monitors with response time', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('HB One');
    await page.locator('#url').fill('https://hb-one.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
              SELECT id, NOW(), true, 42 FROM monitor_urls WHERE url = 'https://hb-one.example.com'`,
    });

    const monitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = 'https://hb-one.example.com'`,
    });
    const monitorID = (monitor.rows as string[][])[0][0];

    await page.goto('/admin/heartbeats');
    await expect(page.getByRole('heading', { name: 'Heartbeats' })).toBeVisible();
    await expect(page.getByRole('link', { name: monitorID, exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: monitorID, exact: true })).toHaveAttribute(
      'href',
      `/admin/monitors/${monitorID}`,
    );
    await expect(page.getByText('HB One')).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Up', exact: true })).toBeVisible();
    await expect(page.getByRole('cell', { name: '42 ms' })).toBeVisible();
  });
});

test.describe('Incidents', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'incidents' });
    await apiCall(request, 'clear-table', { table: 'monitor_checks' });
    await apiCall(request, 'clear-table', { table: 'monitor_urls' });
  });

  test('shows monitor id as a link to the monitor', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Incident Monitor');
    await page.locator('#url').fill('https://incident-link.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO incidents (monitor_url_id, started_at, error_message)
              SELECT id, NOW(), 'timeout' FROM monitor_urls WHERE url = 'https://incident-link.example.com'`,
    });

    const monitor = await apiCall(request, 'sql', {
      query: `SELECT id::text FROM monitor_urls WHERE url = 'https://incident-link.example.com'`,
    });
    const monitorID = (monitor.rows as string[][])[0][0];

    await page.goto('/admin/incidents');
    await expect(page.getByRole('heading', { name: 'Incidents' })).toBeVisible();
    const monitorLink = page.locator('.incidents-table tbody tr').first().getByRole('link', {
      name: monitorID,
      exact: true,
    });
    await expect(monitorLink).toHaveAttribute('href', `/admin/monitors/${monitorID}`);
    await monitorLink.click();
    await expect(page).toHaveURL(`/admin/monitors/${monitorID}`);
    await expect(page.getByRole('heading', { name: 'Incident Monitor' })).toBeVisible();
  });
});

test.describe('Application logs', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
  });

  test('shows application events instead of HTTP access logs', async ({ page }) => {
    // Dev tools генерирует applog event; на /admin/logs не должно быть GET /admin/logs access log.
    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test event' }).click();
    await expect(page.getByText(/Test event recorded:/)).toBeVisible();

    await page.goto('/admin/logs');
    await expect(page.getByRole('heading', { name: 'Application Logs' })).toBeVisible();
    await expect(page.locator('.log-table tbody').getByText('test event from dev tools')).toBeVisible();
    await expect(page.locator('.log-table tbody').getByText('GET /admin/logs')).not.toBeVisible();
  });

  test('shows newest events first', async ({ page }) => {
    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test event' }).click();
    await expect(page.getByText(/Test event recorded:/)).toBeVisible();

    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test event' }).click();
    await expect(page.getByText(/Test event recorded:/)).toBeVisible();

    await page.goto('/admin/logs');
    const testRows = page.locator('.log-table tbody tr', { hasText: 'test event from dev tools' });
    await expect(testRows.first()).toBeVisible();
    const count = await testRows.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Время в первой колонке — по убыванию (новые сверху).
    const times: string[] = [];
    for (let i = 0; i < Math.min(count, 5); i++) {
      times.push((await testRows.nth(i).locator('td').first().textContent()) ?? '');
    }
    for (let i = 0; i < times.length - 1; i++) {
      expect(times[i] >= times[i + 1]).toBeTruthy();
    }
  });
});

test.describe('Monitor requests', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
  });

  test('shows in-memory monitor HTTP requests newest first', async ({ page }) => {
    await page.goto('/admin/requests');
    await expect(page.getByRole('heading', { name: 'Monitor Requests' })).toBeVisible();
    await expect(page.locator('.requests-table tbody')).toBeVisible();
  });
});

test.describe('Settings', () => {
  test('updates check interval', async ({ page, request }) => {
    await login(page, request);

    await page.goto('/admin/settings');
    await page.locator('#check_interval_seconds').fill('120');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page).toHaveURL('/admin/settings');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.locator('#check_interval_seconds')).toHaveValue('120');
  });
});

test.describe('Dev tools', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await apiCall(request, 'clear-table', { table: 'app_settings' });
  });

  test('disables notification test buttons without saved settings', async ({ page }) => {
    await page.goto('/admin/tools');
    await expect(page.getByRole('button', { name: 'Send test Telegram' })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Send test email' })).toBeDisabled();
    await expect(page.getByText('Buttons are enabled only when the corresponding channel is saved')).toBeVisible();
  });

  test('enables notification test buttons when settings are saved', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.locator('#notification_telegram_url').fill('telegram://token@telegram?channels=123');
    await page.locator('#notification_smtp_host').fill('smtp.example.com');
    await page.locator('#notification_smtp_to').fill('ops@example.com');
    await page.getByRole('button', { name: 'Save' }).click();

    await page.goto('/admin/tools');
    await expect(page.getByRole('button', { name: 'Send test Telegram' })).toBeEnabled();
    await expect(page.getByRole('button', { name: 'Send test email' })).toBeEnabled();
  });

  test('generates test error visible on errors page', async ({ page }) => {
    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test error' }).click();
    await expect(page.getByText('Test error recorded. Open Errors to view it.')).toBeVisible();

    await page.goto('/admin/errors');
    await expect(page.getByText('test error from dev tools')).toBeVisible();
    await expect(page.getByText('manual trigger')).toBeVisible();
  });
});
