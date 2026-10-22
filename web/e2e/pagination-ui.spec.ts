import { test, expect } from "@playwright/test";
import { api, getE2EAdminToken, E2E_SEEDED_ADMIN_EMAIL, TournamentRequest, Tournament } from "./helpers";

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

// The admin table shows 20 rows per page, so a second page only exists
// once more than 20 tournaments are stored. How many the scraper has
// written by the time the suite runs depends on the clock, so the
// rows the pagination needs are seeded here instead of being waited
// for.
const SEEDED_TOURNAMENTS = 25;

test.describe("Pagination URL sync", () => {
  test.beforeAll(async ({ request }) => {
    const accessToken = getE2EAdminToken();
    for (let i = 0; i < SEEDED_TOURNAMENTS; i++) {
      const slug = `e2e-page-${String(i).padStart(3, "0")}`;
      const resp = await api.post<Tournament, TournamentRequest>(
        request,
        "/api/web/v1/tournaments",
        { name: `E2E Page ${String(i).padStart(3, "0")}`, slug },
        TournamentRequest,
        Tournament,
        { Authorization: `Bearer ${accessToken}` },
      );
      if (resp.status !== 201) {
        throw new Error(`could not seed tournament ${slug}: ${resp.error ?? resp.status}`);
      }
    }
  });

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

test.describe("Events page filters", () => {
  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("DESC toggle persists direction=desc in URL and survives reload", async ({ page }) => {
    await page.goto("/#/events");
    await page.getByRole("button", { name: /DESC/ }).click();
    await expect(page).toHaveURL(/direction=desc/);

    await page.reload();
    await expect(page).toHaveURL(/direction=desc/);
  });

  test("sport dropdown updates URL and reload preserves it", async ({ page }) => {
    await page.goto("/#/events");
    await page.locator("select").first().selectOption("football");
    await expect(page).toHaveURL(/sport=football/);

    await page.reload();
    await expect(page).toHaveURL(/sport=football/);
  });

  test("tz appears in URL on first load", async ({ page }) => {
    await page.goto("/#/events");
    await expect(page).toHaveURL(/tz=/);
  });
});
