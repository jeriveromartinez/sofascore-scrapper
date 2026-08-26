import { test, expect } from "@playwright/test";
import { getE2EAdminToken } from "./helpers";

const USER_INFO_KEY = "user_info";

async function injectAdminSession(page: import("@playwright/test").Page): Promise<void> {
  const token = getE2EAdminToken();
  const user = {
    id: 0,
    email: "e2e-admin@test.local",
    token,
    refreshToken: token,
  };
  await page.addInitScript(
    ([key, value]) => {
      localStorage.setItem(key, value);
    },
    [USER_INFO_KEY, JSON.stringify(user)],
  );
}

test.describe("Pagination URL sync", () => {
  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("clicking Siguiente updates ?page=2 in the URL", async ({ page }) => {
    await page.goto("/#/tournaments");
    await page.click('button:has-text("Siguiente")');
    await page.waitForURL(/[?&]page=2/);
  });

  test("refreshing on a non-first page restores the same data", async ({ page }) => {
    await page.goto("/#/tournaments?page=2&size=20");
    await page.waitForSelector("table tbody tr");
    const firstRow = await page.locator("table tbody tr").first().textContent();
    await page.reload();
    await page.waitForSelector("table tbody tr");
    const firstRowAfter = await page.locator("table tbody tr").first().textContent();
    expect(firstRowAfter).toBe(firstRow);
  });

  test("invalid ?page is normalised to 1", async ({ page }) => {
    await page.goto("/#/tournaments?page=abc&size=10");
    await page.waitForURL(/[?&]page=1/);
  });
});