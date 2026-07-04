import { test, expect, APIRequestContext } from '@playwright/test';

async function apiCall(request: APIRequestContext, endpoint: string, data: Record<string, unknown>) {
  const response = await request.post(`/api/playwright/${endpoint}`, { data });
  const text = await response.text();
  expect(response.ok(), `API call failed: ${text}`).toBeTruthy();
  return JSON.parse(text);
}

test.describe('Authentication', () => {
  test.beforeEach(async ({ request }) => {
    await apiCall(request, 'clear-table', { table: 'users' });
  });

  test('redirects unauthenticated users to login', async ({ page }) => {
    await page.goto('/admin/users');
    await expect(page).toHaveURL('/login');
  });

  test('logs in and out successfully', async ({ page, request }) => {
    await apiCall(request, 'create-user', {
      username: 'testuser',
      password: 'password123',
    });

    await page.goto('/login');
    await page.locator('#username').fill('testuser');
    await page.locator('#password').fill('password123');
    await page.getByRole('button', { name: 'Login' }).click();

    await expect(page).toHaveURL('/admin/');
    await expect(page.getByRole('heading', { name: 'Admin Dashboard' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();

    await page.getByRole('button', { name: 'Logout' }).click();
    await expect(page).toHaveURL('/login');
  });

  test('shows error on invalid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.locator('#username').fill('wrong');
    await page.locator('#password').fill('wrong');
    await page.getByRole('button', { name: 'Login' }).click();

    await expect(page).toHaveURL('/login');
    await expect(page.getByText('Invalid username or password')).toBeVisible();
  });
});
