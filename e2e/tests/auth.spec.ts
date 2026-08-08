import { test, expect, APIRequestContext } from '@playwright/test';

// E2e-тесты аутентификации: редирект неавторизованных, вход/выход, неверные credentials.
// Данные подготавливаются через /api/playwright (доступен только при GO_UPTIME_ENABLE_PLAYWRIGHT_API).

/** POST к dev-only Playwright API; падает тест, если сервер вернул не 2xx. */
async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown>) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

test.describe('Authentication', () => {
  test.beforeEach(async ({ request }) => {
    // Чистая таблица users перед каждым тестом — нет пользователей «с прошлого» прогона.
    await apiCall(request, 'clear-table', { table: 'users' });
  });

  test('redirects unauthenticated users to login', async ({ page }) => {
    // Защищённый маршрут без сессии → редирект на /login.
    await page.goto('/admin/users');
    await expect(page).toHaveURL('/login');
  });

  test('logs in and out successfully', async ({ page, request }) => {
    // Seed: пользователь для входа через UI.
    await apiCall(request, 'create-user', {
      username: 'testuser',
      password: 'password123',
    });

    // Вход через форму логина.
    await page.goto('/login');
    await page.locator('#username').fill('testuser');
    await page.locator('#password').fill('password123');
    await page.getByRole('button', { name: 'Login' }).click();

    // После входа — админка, заголовок и кнопка Logout.
    await expect(page).toHaveURL('/admin/');
    await expect(page.getByRole('heading', { name: 'Admin Dashboard' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();

    // Выход возвращает на страницу логина.
    await page.getByRole('button', { name: 'Logout' }).click();
    await expect(page).toHaveURL('/login');
  });

  test('shows error on invalid credentials', async ({ page }) => {
    // Несуществующий пользователь — остаёмся на /login с сообщением об ошибке.
    await page.goto('/login');
    await page.locator('#username').fill('wrong');
    await page.locator('#password').fill('wrong');
    await page.getByRole('button', { name: 'Login' }).click();

    await expect(page).toHaveURL('/login');
    await expect(page.getByText('Invalid username or password')).toBeVisible();
  });
});
