import { test, expect } from "@playwright/test";
import { api, AuthRequest, AuthResponse, TournamentRequest, Tournament, TournamentList, StatusMessage } from "./helpers";

const AUTH_PATH = "/api/web/v1/users";
const TOURNAMENTS_PATH = "/api/web/v1/tournaments";

const TEST_EMAIL = `e2e-tourn-${Date.now()}@test.local`;
const TEST_PASSWORD = "Password1!";

let accessToken: string;

test.describe("Tournament CRUD", () => {
  test.beforeAll(async ({ request }) => {
    const bootstrapToken = await getBootstrapInvitationToken();
    const registerResp = await api.post<AuthResponse, AuthRequest>(
      request,
      `${AUTH_PATH}/register`,
      { email: TEST_EMAIL, password: TEST_PASSWORD, invitationToken: bootstrapToken },
      AuthRequest,
      AuthResponse,
    );
    expect(registerResp.status).toBe(201);
    accessToken = registerResp.data!.token;
  });

  test("list tournaments (empty initially)", async ({ request }) => {
    const resp = await api.get<TournamentList>(
      request,
      TOURNAMENTS_PATH,
      TournamentList,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(200);
    expect(resp.data!.tournaments).toEqual([]);
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

async function getBootstrapInvitationToken(): Promise<string> {
  const { execSync } = await import("child_process");
  const composeFile = "deployments/docker/compose.test.yml";
  try {
    return execSync(
      `docker compose -f ${composeFile} exec -T backend cat /shared/invite.txt`,
      { encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
    ).toString().trim();
  } catch {
    throw new Error("Could not read bootstrap invitation token.");
  }
}
