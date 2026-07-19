import { test, expect } from "@playwright/test";
import { api, getE2EAdminToken, StatusMessage, TournamentRequest, Tournament, TournamentPage } from "./helpers";

const TOURNAMENTS_PATH = "/api/web/v1/tournaments";
const PAGE_PATH = "/api/web/v1/tournaments/page";

let accessToken: string;
const tournamentIds: number[] = [];

test.describe("Cursor pagination", () => {
  test.beforeAll(async ({ request }) => {
    accessToken = getE2EAdminToken();
    for (let i = 0; i < 25; i++) {
      const response = await api.post<Tournament, TournamentRequest>(
        request,
        TOURNAMENTS_PATH,
        { name: `Tournament ${String(i).padStart(3, "0")}`, slug: `tournament-${String(i).padStart(3, "0")}` },
        TournamentRequest,
        Tournament,
        { Authorization: `Bearer ${accessToken}` },
      );
      if (response.status !== 201 || !response.data) {
        throw new Error(`Could not seed pagination tournament: ${response.error ?? response.status}`);
      }
      tournamentIds.push(response.data.id);
    }
  });

  test.afterAll(async ({ request }) => {
    for (const id of tournamentIds) {
      await api.delete(request, `${TOURNAMENTS_PATH}/${id}`, StatusMessage, {
        Authorization: `Bearer ${accessToken}`,
      });
    }
  });

  test("first page returns data with next_cursor", async ({ request }) => {
    const resp = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?limit=10`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status, `page error: ${resp.error}`).toBe(200);
    expect(resp.data!.data.length).toBe(10);
    expect(resp.data!.page!.nextCursor).toBeTruthy();
    expect(resp.data!.page!.hasMore).toBe(true);
  });

  test("second page via cursor returns next batch", async ({ request }) => {
    const page1 = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?limit=10`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(page1.status).toBe(200);
    expect(page1.data!.page!.nextCursor).toBeTruthy();

    const page2 = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?limit=10&cursor=${encodeURIComponent(page1.data!.page!.nextCursor)}`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(page2.status).toBe(200);
    expect(page2.data!.data.length).toBe(10);

    // Ensure second page has different items
    const firstId = page1.data!.data[0].id;
    const secondFirstId = page2.data!.data[0].id;
    expect(secondFirstId).not.toBe(firstId);
  });

  test("last page returns hasMore=false and no next_cursor", async ({ request }) => {
    // limit=10, 25 items → page1(10), page2(10), page3(5)
    const page1 = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?limit=10`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(page1.status).toBe(200);

    const page2 = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?limit=10&cursor=${encodeURIComponent(page1.data!.page!.nextCursor)}`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(page2.status).toBe(200);

    if (page2.data!.page!.hasMore) {
      const page3 = await api.get<TournamentPage>(
        request,
        `${PAGE_PATH}?limit=10&cursor=${encodeURIComponent(page2.data!.page!.nextCursor)}`,
        TournamentPage,
        { Authorization: `Bearer ${accessToken}` },
      );
      expect(page3.status).toBe(200);
      expect(page3.data!.page!.hasMore).toBe(false);
      expect(page3.data!.page!.nextCursor).toBe("");
    }
  });

  test("invalid cursor returns 400", async ({ request }) => {
    const resp = await api.get<TournamentPage>(
      request,
      `${PAGE_PATH}?cursor=invalid_base64!!!`,
      TournamentPage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(400);
  });
});
