import { test, expect } from "@playwright/test";
import { api, getE2EAdminToken, TournamentRequest, Tournament, TournamentList, StatusMessage } from "./helpers";

const TOURNAMENTS_PATH = "/api/web/v1/tournaments";

let accessToken: string;

// The tournaments this file creates are removed again, so a run does
// not leave rows behind that the next run has to page around.
const createdIds: number[] = [];

test.describe("Tournament CRUD", () => {
  test.beforeAll(() => {
    accessToken = getE2EAdminToken();
  });

  test.afterAll(async ({ request }) => {
    for (const id of createdIds) {
      const resp = await api.delete<StatusMessage>(
        request,
        `${TOURNAMENTS_PATH}/${id}`,
        StatusMessage,
        { Authorization: `Bearer ${accessToken}` },
      );
      expect(resp.status, `delete tournament error: ${resp.error}`).toBe(200);
    }
  });

  test("list tournaments returns the created tournament", async ({ request }) => {
    // The list is not empty on a running stack: the scraper seeds the
    // curated leagues on the first boot. What matters is that a
    // tournament created through the API shows up in the list.
    const created = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "Listed Tournament", slug: "listed-tournament" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(created.status, `create error: ${created.error}`).toBe(201);
    createdIds.push(created.data!.id);

    const resp = await api.get<TournamentList>(
      request,
      TOURNAMENTS_PATH,
      TournamentList,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(200);
    const tournaments = resp.data!.tournaments;
    expect(Array.isArray(tournaments)).toBe(true);
    expect(tournaments.some((t) => t.id === created.data!.id)).toBe(true);
  });

  test("create tournament", async ({ request }) => {
    const resp = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "Premier League", slug: "premier-league" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status, `create error: ${resp.error}`).toBe(201);
    createdIds.push(resp.data!.id);
    expect(resp.data!.name).toBe("Premier League");
    expect(resp.data!.slug).toBe("premier-league");
    expect(resp.data!.id).toBeGreaterThan(0);
  });

  test("create tournament without name fails", async ({ request }) => {
    const resp = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "", slug: "empty" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(400);
  });

  test("get tournament by id", async ({ request }) => {
    const createResp = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "La Liga", slug: "la-liga" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(createResp.status).toBe(201);
    const id = createResp.data!.id;
    createdIds.push(id);

    const getResp = await api.get<Tournament>(
      request,
      `${TOURNAMENTS_PATH}/${id}`,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(getResp.status).toBe(200);
    expect(getResp.data!.name).toBe("La Liga");
    expect(getResp.data!.slug).toBe("la-liga");
  });

  test("get non-existent tournament returns 404", async ({ request }) => {
    const resp = await api.get<Tournament>(
      request,
      `${TOURNAMENTS_PATH}/99999`,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(404);
  });

  test("update tournament", async ({ request }) => {
    const createResp = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "Bundesliga", slug: "bundesliga" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(createResp.status).toBe(201);
    const id = createResp.data!.id;
    createdIds.push(id);

    const updateResp = await api.put<Tournament, TournamentRequest>(
      request,
      `${TOURNAMENTS_PATH}/${id}`,
      { name: "Bundesliga Updated", slug: "bundesliga-updated" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(updateResp.status).toBe(200);
    expect(updateResp.data!.name).toBe("Bundesliga Updated");
    expect(updateResp.data!.slug).toBe("bundesliga-updated");
  });

  test("delete tournament", async ({ request }) => {
    const createResp = await api.post<Tournament, TournamentRequest>(
      request,
      TOURNAMENTS_PATH,
      { name: "Serie A", slug: "serie-a" },
      TournamentRequest,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(createResp.status).toBe(201);
    const id = createResp.data!.id;

    const deleteResp = await api.delete<StatusMessage>(
      request,
      `${TOURNAMENTS_PATH}/${id}`,
      StatusMessage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(deleteResp.status).toBe(200);
    expect(deleteResp.data!.message).toContain("deleted");

    const getResp = await api.get<Tournament>(
      request,
      `${TOURNAMENTS_PATH}/${id}`,
      Tournament,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(getResp.status).toBe(404);
  });

  test("list tournaments after CRUD", async ({ request }) => {
    const resp = await api.get<TournamentList>(
      request,
      TOURNAMENTS_PATH,
      TournamentList,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(200);
    expect(resp.data!.tournaments.length).toBeGreaterThanOrEqual(2);
  });

  test("unauth cannot access tournaments", async ({ request }) => {
    const resp = await api.get<TournamentList>(
      request,
      TOURNAMENTS_PATH,
      TournamentList,
    );
    expect(resp.status).toBe(401);
  });
});
