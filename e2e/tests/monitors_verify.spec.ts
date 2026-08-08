import { test, expect, APIRequestContext } from '@playwright/test';

// E2e-тесты опции «Verify before create»: HTTP-probe URL до сохранения монитора (single и bulk).
// Probe выполняется из контейнера app — localhost:8080, не проброшенный на хост порт 18081.

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

/** Очистка таблиц мониторов и связанных данных перед тестом verify. */
async function clearMonitors(request: APIRequestContext) {
  await apiCall(request, 'clear-table', { table: 'incidents' });
  await apiCall(request, 'clear-table', { table: 'monitor_checks' });
  await apiCall(request, 'clear-table', { table: 'monitor_urls' });
}

test.describe('Verify before create', () => {
  test.beforeEach(async ({ page, request }) => {
    await login(page, request);
    await clearMonitors(request);
  });

  test('shows verify checkbox on single and bulk forms', async ({ page }) => {
    // Чекбокс на форме одного монитора — по умолчанию выключен.
    await page.goto('/admin/monitors/new');
    await expect(page.locator('#verify_before_create')).toBeVisible();
    await expect(page.locator('#verify_before_create')).not.toBeChecked();

    // То же на bulk-форме.
    await page.goto('/admin/monitors/bulk/new');
    await expect(page.locator('#verify_before_create')).toBeVisible();
    await expect(page.locator('#verify_before_create')).not.toBeChecked();
  });

  test('rejects unavailable URL when verify is checked on single form', async ({ page, request }) => {
    // Порт 1 на 127.0.0.1 в контейнере недоступен — ожидаем ошибку, монитор не создаётся.
    await page.goto('/admin/monitors/new');
    await page.locator('#url').fill('http://127.0.0.1:1');
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.locator('.alert-danger')).toContainText('unavailable');
    await expect(page.locator('#verify_before_create')).toBeChecked();
    await expect(page.locator('#url')).toHaveValue('http://127.0.0.1:1');

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('0');
  });

  test('creates reachable URL when verify is checked on single form', async ({ page }) => {
    // Probe идёт из контейнера приложения, где слушается 8080 (не проброшенный на хост 18081).
    const reachableURL = 'http://127.0.0.1:8080/health';

    await page.goto('/admin/monitors/new');
    await page.locator('#name').fill('Local Health');
    await page.locator('#url').fill(reachableURL);
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    await expect(page.getByText(reachableURL)).toBeVisible();
  });

  test('rejects bulk create when verify is checked and any URL is unavailable', async ({ page, request }) => {
    // Смешанный список: health OK, порт 1 — fail; транзакция не должна создать ни одной записи.
    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill('http://127.0.0.1:8080/health\nhttp://127.0.0.1:1');
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors/bulk');
    await expect(page.locator('.alert-danger')).toContainText('unavailable');
    await expect(page.locator('#verify_before_create')).toBeChecked();

    const result = await apiCall(request, 'sql', {
      query: `SELECT COUNT(*)::text AS count FROM monitor_urls`,
    });
    expect(result.rows[0][0]).toBe('0');
  });

  test('creates bulk monitors when verify is checked and all URLs are reachable', async ({ page }) => {
    // Оба URL — эндпоинты того же app внутри контейнера (8080).
    const urls = ['http://127.0.0.1:8080/health', 'http://127.0.0.1:8080/login'];

    await page.goto('/admin/monitors/bulk/new');
    await page.locator('#urls').fill(urls.join('\n'));
    await page.locator('#verify_before_create').check();
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page).toHaveURL('/admin/monitors');
    await expect(page.getByText('Saved successfully.')).toBeVisible();
    for (const url of urls) {
      await expect(page.getByText(url)).toBeVisible();
    }
  });
});
