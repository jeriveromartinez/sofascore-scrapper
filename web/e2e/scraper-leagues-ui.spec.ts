import { test, expect } from "@playwright/test";
import { getE2EAdminToken, getE2EState, E2E_ADMIN_EMAIL } from "./helpers";

const USER_INFO_KEY = "user_info";
const LEAGUES_PATH = "/api/web/v1/scraper-leagues";

// The tests below need a row in the table. The spec seeds a league of
// its own and removes it again in the teardown, so the table is never
// empty and the leagues of the stack are left alone.
const SEEDED_LEAGUE = {
  source: "fotmob",
  source_league_id: `e2e-${Date.now()}`,
  name: `E2E Scraper League ${Date.now()}`,
  country: "ES",
  sport: "football",
};

let seededLeagueId = 0;

async function injectAdminSession(page: import("@playwright/test").Page): Promise<void> {
  const token = getE2EAdminToken();
  const user = {
    id: getE2EState().adminUserId,
    email: E2E_ADMIN_EMAIL,
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
  test.beforeAll(async ({ request }) => {
    const resp = await request.post(LEAGUES_PATH, {
      headers: {
        Authorization: `Bearer ${getE2EAdminToken()}`,
        "Content-Type": "application/json",
      },
      data: SEEDED_LEAGUE,
    });
    const body = (await resp.json().catch(() => null)) as { ID?: number; id?: number } | null;
    expect(resp.status(), `create league error: ${JSON.stringify(body)}`).toBe(201);
    const id = body?.ID ?? body?.id;
    expect(id, `the created league has no id: ${JSON.stringify(body)}`).toBeGreaterThan(0);
    seededLeagueId = id!;
  });

  test.afterAll(async ({ request }) => {
    if (!seededLeagueId) return;
    const resp = await request.delete(`${LEAGUES_PATH}/${seededLeagueId}`, {
      headers: { Authorization: `Bearer ${getE2EAdminToken()}` },
    });
    expect(resp.status(), `delete league error: ${resp.status()}`).toBe(204);
  });

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
    await page.waitForLoadState("networkidle");
    const editButtons = page.locator("button.edit");
    expect(await editButtons.count(), "the table holds no league to edit").toBeGreaterThan(0);
    await editButtons.first().click();
    await expect(page.locator(".modal h2")).toContainText("Edit league");
  });

  test("delete button on a row opens ConfirmDelete modal", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const deleteButtons = page.locator("button.delete");
    expect(await deleteButtons.count(), "the table holds no league to delete").toBeGreaterThan(0);
    await deleteButtons.first().click();
    await expect(page.locator(".modal-backdrop")).toBeVisible();
  });

  test("cancel button in ConfirmDelete closes the modal without deleting", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const deleteButtons = page.locator("button.delete");
    expect(await deleteButtons.count(), "the table holds no league to delete").toBeGreaterThan(0);
    await deleteButtons.first().click();
    await page.click('.modal-backdrop .modal button:has-text("Cancel")');
    await expect(page.locator(".modal-backdrop")).toHaveCount(0);
  });

  test("status toggle calls the update endpoint", async ({ page }) => {
    await page.goto("/#/scraper-leagues");
    await page.waitForLoadState("networkidle");
    const toggles = page.locator("button.status-toggle");
    expect(await toggles.count(), "the table holds no league to toggle").toBeGreaterThan(0);
    // Snapshot the first toggle's text (ON or OFF), click, assert it changed
    const initial = await toggles.first().textContent();
    await toggles.first().click();
    await expect(toggles.first()).not.toHaveText(initial!);
    // Restore the original state: the suite runs against a persistent backend,
    // so leaving the league flipped would change the state of every later run.
    await toggles.first().click();
    await expect(toggles.first()).toHaveText(initial!);
  });
});
