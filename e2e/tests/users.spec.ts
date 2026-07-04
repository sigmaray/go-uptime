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
  await expect(page).toHaveURL('/admin/');
}

test.describe('Users CRUD', () => {
  test('creates, edits and deletes a user', async ({ page, request }) => {
    await login(page, request);

    await page.goto('/admin/users');
    await page.getByRole('link', { name: 'Create User' }).click();
    await page.locator('#username').fill('newuser');
    await page.locator('#password').fill('secret123');
    await page.locator('#confirm_password').fill('secret123');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/users?saved=1');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('newuser')).toBeVisible();

    await page.getByRole('row', { name: /newuser/ }).getByRole('link', { name: 'Edit' }).click();
    await page.locator('#username').fill('renamed');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page).toHaveURL('/admin/users?saved=1');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('renamed')).toBeVisible();

    page.on('dialog', (dialog) => dialog.accept());
    await page.getByRole('row', { name: /renamed/ }).getByRole('button', { name: 'Delete' }).click();
    await expect(page.getByText('renamed')).not.toBeVisible();
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

    await expect(page).toHaveURL('/admin/monitors?saved=1');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText('https://example.com')).toBeVisible();
  });

  test('uses URL hostname when name is omitted', async ({ page }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('https://example.org');
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors?saved=1');
    await expect(page.getByRole('cell', { name: 'example.org', exact: true })).toBeVisible();
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
    await page.getByRole('link', { name: 'Detail Test' }).click();

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

  test('lists heartbeats from all monitors', async ({ page, request }) => {
    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('HB One');
    await page.locator('#url').fill('https://hb-one.example.com');
    await page.getByRole('button', { name: 'Create' }).click();

    await apiCall(request, 'sql', {
      query: `INSERT INTO monitor_checks (monitor_url_id, checked_at, is_up)
              SELECT id, NOW(), true FROM monitor_urls WHERE url = 'https://hb-one.example.com'`,
    });

    await page.goto('/admin/heartbeats');
    await expect(page.getByRole('heading', { name: 'Heartbeats' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'HB One' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Up', exact: true })).toBeVisible();
  });
});

test.describe('Application logs', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
  });

  test('shows zerolog entries including HTTP access logs', async ({ page }) => {
    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test log' }).click();
    await expect(page.getByText(/Test log recorded:/)).toBeVisible();

    await page.goto('/admin/logs');
    await expect(page.getByRole('heading', { name: 'Application Logs' })).toBeVisible();
    await expect(page.locator('.log-table tbody').getByText('test log entry from dev tools')).toBeVisible();
  });

  test('shows newest log entries first', async ({ page }) => {
    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test log' }).click();
    await expect(page.getByText(/Test log recorded:/)).toBeVisible();

    await page.goto('/admin/tools');
    await page.getByRole('button', { name: 'Generate test log' }).click();
    await expect(page.getByText(/Test log recorded:/)).toBeVisible();

    await page.goto('/admin/logs');
    const testRows = page.locator('.log-table tbody tr', { hasText: 'test log entry from dev tools' });
    await expect(testRows.first()).toBeVisible();
    const count = await testRows.count();
    expect(count).toBeGreaterThanOrEqual(2);

    const times: string[] = [];
    for (let i = 0; i < Math.min(count, 5); i++) {
      times.push((await testRows.nth(i).locator('td').first().textContent()) ?? '');
    }
    for (let i = 0; i < times.length - 1; i++) {
      expect(times[i] >= times[i + 1]).toBeTruthy();
    }
  });
});

test.describe('Settings', () => {
  test('updates check interval', async ({ page, request }) => {
    await login(page, request);

    await page.goto('/app/settings');
    await page.locator('#check_interval_seconds').fill('120');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page).toHaveURL('/app/settings?saved=1');
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
    await page.goto('/app/settings');
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
  });
});
