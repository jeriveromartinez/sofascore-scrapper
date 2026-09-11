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

test.describe("Admin > Scraper Leagues", () => {
  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("page loads and shows the table heading", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await expect(page.locator("h1")).toContainText("Scraper Leagues");
  });

  test("add-new button opens the CreateOrEdit modal in create mode", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.click("button.add-new");
    await expect(page.locator(".modal h2")).toContainText("Add new league");
  });

  test("edit button on a row opens the CreateOrEdit modal in edit mode", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    // wait for table to load (at least one row, or empty state — be tolerant)
    await page.waitForLoadState("networkidle");
    const editButtons = page.locator("button.edit");
    const count = await editButtons.count();
    test.skip(count === 0, "no rows to edit (empty backend)");
    await editButtons.first().click();
    await expect(page.locator(".modal h2")).toContainText("Edit league");
  });

  test("delete button on a row opens ConfirmDelete modal", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const deleteButtons = page.locator("button.delete");
    const count = await deleteButtons.count();
    test.skip(count === 0, "no rows to delete (empty backend)");
    await deleteButtons.first().click();
    await expect(page.locator(".modal-backdrop")).toBeVisible();
  });

  test("cancel button in ConfirmDelete closes the modal without deleting", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const deleteButtons = page.locator("button.delete");
    const count = await deleteButtons.count();
    test.skip(count === 0, "no rows to delete (empty backend)");
    await deleteButtons.first().click();
    await page.click('.modal-backdrop .modal button:has-text("Cancel")');
    await expect(page.locator(".modal-backdrop")).toHaveCount(0);
  });

  test("status toggle calls the update endpoint", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const toggles = page.locator("button.status-toggle");
    const count = await toggles.count();
    test.skip(count === 0, "no rows to toggle (empty backend)");
    // Snapshot the first toggle's text (ON or OFF), click, assert it changed
    const initial = await toggles.first().textContent();
    await toggles.first().click();
    await expect(toggles.first()).not.toHaveText(initial!);
  });
});
