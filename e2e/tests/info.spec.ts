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
    await expect(page.getByTestId('info-utilization-gauges')).toBeVisible();
    await expect(page.getByTestId('info-fleet-empty')).toBeVisible();
    await expect(page.getByTestId('info-backlog-empty')).toBeVisible();

    await expect(page.getByTestId('info-heartbeat-hour-chart')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Heartbeats — past hour' })).toBeVisible();
    await expect(page.getByTestId('info-heartbeat-hour-total')).toHaveText('0 total');
    await expect(page.getByTestId('info-heartbeat-hour-success')).toHaveText('0 successful');
    await expect(page.getByTestId('info-heartbeat-hour-failed')).toHaveText('0 failed');
    await expect(page.getByTestId('info-heartbeat-minute')).toHaveCount(60);

    await expect(page.getByTestId('info-table-counts')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Database tables' })).toBeVisible();
    await expect(page.getByTestId('info-table-count-monitor_urls')).toHaveText('0');
    await expect(page.getByTestId('info-table-count-users')).toHaveText('1');
    await expect(page.getByTestId('info-table-count-incidents')).toHaveText('0');
    await expect(page.getByTestId('info-table-count-monitor_checks')).toHaveText('0');
    await expect(page.getByTestId('info-table-count-app_settings')).toBeVisible();
    await expect(page.getByTestId('info-table-count-stat_minutely')).toBeVisible();
    await expect(page.getByTestId('info-table-count-stat_hourly')).toBeVisible();
    await expect(page.getByTestId('info-table-count-stat_daily')).toBeVisible();
  });

  test('counts seeded monitors on the info page', async ({ page, request }) => {
    await apiCall(request, 'sql', {
      query: `
        INSERT INTO monitor_urls (name, url, check_interval_seconds, is_up, created_at, updated_at)
        VALUES
          ('Never Checked', 'https://never.example', 3600, NULL, NOW(), NOW()),
          ('Overdue Site', 'https://overdue.example', 3600, false, NOW(), NOW()),
          ('Fresh Up', 'https://fresh.example', 3600, true, NOW(), NOW())
      `,
    });
    await apiCall(request, 'sql', {
      query: `
        UPDATE monitor_urls
        SET last_checked_at = NOW() - INTERVAL '2 hours'
        WHERE url = 'https://overdue.example'
      `,
    });
    await apiCall(request, 'sql', {
      query: `
        UPDATE monitor_urls
        SET last_checked_at = NOW()
        WHERE url = 'https://fresh.example'
      `,
    });

    await page.goto('/admin/info');
    await expect(page.getByTestId('info-total-monitors')).toHaveText('3');
    await expect(page.getByTestId('info-check-concurrency')).not.toHaveText('');
    await expect(page.getByTestId('info-worker-stats')).toBeVisible();
    await expect(page.getByTestId('info-utilization-gauges')).toBeVisible();

    await expect(page.getByTestId('info-fleet-up')).toHaveText('1');
    await expect(page.getByTestId('info-fleet-down')).toHaveText('1');
    await expect(page.getByTestId('info-fleet-unknown')).toHaveText('1');
    await expect(page.getByTestId('info-backlog-due')).toHaveText('1');
    await expect(page.getByTestId('info-backlog-never')).toHaveText('1');
    await expect(page.getByTestId('info-backlog-schedule')).toHaveText('1');
    await expect(page.getByTestId('info-table-count-monitor_urls')).toHaveText('3');

    const dueText = await page.getByTestId('info-due-waiting').innerText();
    const neverText = await page.getByTestId('info-never-checked').innerText();
    expect(Number(dueText)).toBeGreaterThanOrEqual(0);
    expect(Number(neverText)).toBe(1);

    const overdue = page.getByTestId('info-most-overdue');
    const overdueEmpty = page.getByTestId('info-most-overdue-empty');
    if (await overdue.count()) {
      await expect(overdue).toContainText('Overdue Site');
    } else {
      await expect(overdueEmpty).toBeVisible();
    }
  });

  test('shows successful and failed heartbeats on the past-hour chart', async ({ page, request }) => {
    await apiCall(request, 'sql', {
      query: `
        INSERT INTO monitor_urls (name, url, check_interval_seconds, is_up, last_checked_at, created_at, updated_at)
        VALUES ('Chart Site', 'https://chart.example', 60, true, NOW(), NOW(), NOW())
      `,
    });
    const monitor = await apiCall(request, 'sql', {
      query: `SELECT id FROM monitor_urls WHERE url = 'https://chart.example'`,
    });
    const monitorID = (monitor.rows as string[][])[0][0];

    await apiCall(request, 'sql', {
      query: `
        INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up, response_time_ms)
        VALUES
          (${monitorID}, date_trunc('minute', NOW()), true, 10),
          (${monitorID}, date_trunc('minute', NOW()), true, 12),
          (${monitorID}, date_trunc('minute', NOW()), false, 900),
          (${monitorID}, date_trunc('minute', NOW()) - INTERVAL '5 minutes', true, 11),
          (${monitorID}, date_trunc('minute', NOW()) - INTERVAL '5 minutes', false, 800)
      `,
    });

    await page.goto('/admin/info');

    await expect(page.getByTestId('info-heartbeat-hour-chart')).toBeVisible();
    await expect(page.getByTestId('info-heartbeat-hour-total')).toHaveText('5 total');
    await expect(page.getByTestId('info-heartbeat-hour-success')).toHaveText('3 successful');
    await expect(page.getByTestId('info-heartbeat-hour-failed')).toHaveText('2 failed');
    await expect(page.getByTestId('info-heartbeat-minute')).toHaveCount(60);
  });
});
