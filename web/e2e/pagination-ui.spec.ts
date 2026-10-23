import { test, expect } from "@playwright/test";
import {
  api,
  getE2EAdminToken,
  getE2EState,
  E2E_ADMIN_EMAIL,
  StatusMessage,
  TournamentList,
  TournamentRequest,
  Tournament,
} from "./helpers";

const TOURNAMENTS_PATH = "/api/web/v1/tournaments";
const USER_INFO_KEY = "user_info";

// The admin table shows twenty rows per page, so a second page only
// exists once more than twenty tournaments are stored.
const PAGE_SIZE = 20;

const seededIds: number[] = [];

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

test.describe("Pagination URL sync", () => {
  test.beforeAll(async ({ request }) => {
    const accessToken = getE2EAdminToken();
    const list = await api.get<TournamentList>(request, TOURNAMENTS_PATH, TournamentList, {
      Authorization: `Bearer ${accessToken}`,
    });
    expect(list.status, `tournament list error: ${list.error}`).toBe(200);

    // Only the rows the second page is missing are seeded. A stack
    // that already holds more tournaments than a page shows needs none,
    // so the run leaves nothing behind at all.
    const missing = Math.max(0, PAGE_SIZE + 1 - list.data!.tournaments.length);
    for (let i = 0; i < missing; i++) {
      const slug = `e2e-page-${String(i).padStart(3, "0")}`;
      const resp = await api.post<Tournament, TournamentRequest>(
        request,
        TOURNAMENTS_PATH,
        { name: `E2E Page ${String(i).padStart(3, "0")}`, slug },
        TournamentRequest,
        Tournament,
        { Authorization: `Bearer ${accessToken}` },
      );
      if (resp.status !== 201) {
        throw new Error(`could not seed tournament ${slug}: ${resp.error ?? resp.status}`);
      }
      seededIds.push(resp.data!.id);
    }
  });

  test.afterAll(async ({ request }) => {
    const accessToken = getE2EAdminToken();
    for (const id of seededIds) {
      const resp = await api.delete<StatusMessage>(
        request,
        `${TOURNAMENTS_PATH}/${id}`,
        StatusMessage,
        { Authorization: `Bearer ${accessToken}` },
      );
      expect(resp.status, `delete tournament error: ${resp.error}`).toBe(200);
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
