import { test, expect } from "@playwright/test";
import { getE2EAdminToken, E2E_SEEDED_ADMIN_EMAIL } from "./helpers";

const USER_INFO_KEY = "user_info";

async function injectAdminSession(page: import("@playwright/test").Page): Promise<void> {
  const token = getE2EAdminToken();
  const user = {
    id: 0,
    email: E2E_SEEDED_ADMIN_EMAIL,
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

test.describe("Modal flows", () => {
  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("Create tournament via modal", async ({ page }) => {
    // The admin table is ordered by slug and paginated, so the name has
    // to sort into the first page for the row to be visible without
    // paging. The stamp keeps the slug unique across runs.
    const name = `AAA UI Test Tournament ${Date.now()}`;
    await page.goto("/#/tournaments");
    await page.click('button:has-text("Crear")');
    await page.fill('.modal.show input[required]', name);
    await page.click('.modal.show button:has-text("Crear")');
    await expect(page.locator("table")).toContainText(name);
  });

  test("Edit tournament via modal pre-fills the form", async ({ page }) => {
    await page.goto("/#/tournaments");
    const firstEdit = page.locator('button:has-text("Editar")').first();
    await firstEdit.click();
    const nameInput = page.locator('.modal.show input').first();
    await expect(nameInput).not.toHaveValue("");
  });

  test("Cancel modal does not mutate the row", async ({ page }) => {
    await page.goto("/#/tournaments");
    const firstRow = page.locator("table tbody tr").first();
    const firstRowName = await firstRow.textContent();
    await firstRow.locator('button:has-text("Editar")').click();
    const nameInput = page.locator('.modal.show input').first();
    await nameInput.fill("CHANGED VALUE THAT SHOULD NOT PERSIST");
    await page.click('.modal.show button:has-text("Cancelar")');
    const newFirstRowName = await page.locator("table tbody tr").first().textContent();
    expect(newFirstRowName).toBe(firstRowName);
  });
});