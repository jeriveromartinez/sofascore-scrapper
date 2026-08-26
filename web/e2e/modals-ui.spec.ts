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

test.describe("Modal flows", () => {
  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("Create tournament via modal", async ({ page }) => {
    await page.goto("/#/tournaments");
    await page.click('button:has-text("Crear")');
    await page.fill('.modal.show input[required]', "UI Test Tournament");
    await page.click('.modal.show button:has-text("Crear")');
    await expect(page.locator("table")).toContainText("UI Test Tournament");
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