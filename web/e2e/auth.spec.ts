import { test, expect } from "@playwright/test";
import { api, AuthRequest, AuthResponse, createTestInvitation, CreateInvitationRequest, InvitationResponse } from "./helpers";

const INVITE_PATH = "/api/web/v1/users/invitations";
const REGISTER_PATH = "/api/web/v1/users/register";
const LOGIN_PATH = "/api/web/v1/users/login";
const REFRESH_PATH = "/api/web/v1/users/refresh";

const TEST_EMAIL = `e2e-auth-${Date.now()}@test.local`;
const TEST_PASSWORD = "Password1!";
const TEST_EMAIL2 = `e2e-auth2-${Date.now()}@test.local`;

let bootstrapToken: string;
let accessToken: string;
let refreshToken: string;

test.describe("Auth flow", () => {
  test.beforeAll(async ({ request }) => {
    bootstrapToken = await createTestInvitation(request);
    expect(bootstrapToken).toBeTruthy();
  });

  test("register with bootstrap invitation", async ({ request }) => {
    const resp = await api.post<AuthResponse, AuthRequest>(
      request,
      REGISTER_PATH,
      { email: TEST_EMAIL, password: TEST_PASSWORD, invitationToken: bootstrapToken },
      AuthRequest,
      AuthResponse,
    );
    expect(resp.status, `register error: ${resp.error}`).toBe(201);
    expect(resp.data!.email).toBe(TEST_EMAIL);
    expect(resp.data!.token).toBeTruthy();
    expect(resp.data!.refreshToken).toBeTruthy();
    accessToken = resp.data!.token;
    refreshToken = resp.data!.refreshToken;
  });

  test("login with registered user", async ({ request }) => {
    const resp = await api.post<AuthResponse, AuthRequest>(
      request,
      LOGIN_PATH,
      { email: TEST_EMAIL, password: TEST_PASSWORD, invitationToken: "" },
      AuthRequest,
      AuthResponse,
    );
    expect(resp.status, `login error: ${resp.error}`).toBe(200);
    expect(resp.data!.email).toBe(TEST_EMAIL);
    accessToken = resp.data!.token;
    refreshToken = resp.data!.refreshToken;
  });

  test("login with wrong password returns 401", async ({ request }) => {
    const resp = await api.post<AuthResponse, AuthRequest>(
      request,
      LOGIN_PATH,
      { email: TEST_EMAIL, password: "wrong-pass", invitationToken: "" },
      AuthRequest,
      AuthResponse,
    );
    expect(resp.status).toBe(401);
  });

  test("refresh token gets new token pair", async ({ request }) => {
    expect(refreshToken).toBeTruthy();
    const resp = await api.postNoBody<AuthResponse>(
      request,
      REFRESH_PATH,
      AuthResponse,
      { Authorization: `Bearer ${refreshToken}` },
    );
    expect(resp.status, `refresh error: ${resp.error}`).toBe(200);
    expect(resp.data!.token).toBeTruthy();
    expect(resp.data!.refreshToken).toBeTruthy();
    let newRefreshToken = resp.data!.refreshToken;

    // Old refresh token should be invalidated after rotation
    const retryResp = await api.postNoBody<AuthResponse>(
      request,
      REFRESH_PATH,
      AuthResponse,
      { Authorization: `Bearer ${refreshToken}` },
    );
    expect(retryResp.status).toBe(401);

    refreshToken = newRefreshToken;
    accessToken = resp.data!.token;
  });

  test("authenticated user can create invitation", async ({ request }) => {
    expect(accessToken).toBeTruthy();
    const resp = await api.post<InvitationResponse, CreateInvitationRequest>(
      request,
      INVITE_PATH,
      { ttlSeconds: 600 },
      CreateInvitationRequest,
      InvitationResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status, `invite error: ${resp.error}`).toBe(201);
    expect(resp.data!.token).toBeTruthy();
    expect(resp.data!.expiresAt).toBeGreaterThan(Date.now() / 1000);
  });

  test("unauth cannot create invitation", async ({ request }) => {
    const resp = await api.post<InvitationResponse, CreateInvitationRequest>(
      request,
      INVITE_PATH,
      { ttlSeconds: 600 },
      CreateInvitationRequest,
      InvitationResponse,
    );
    expect(resp.status).toBe(401);
  });

  test("register without invitation token fails", async ({ request }) => {
    const resp = await api.post<AuthResponse, AuthRequest>(
      request,
      REGISTER_PATH,
      { email: TEST_EMAIL2, password: TEST_PASSWORD, invitationToken: "" },
      AuthRequest,
      AuthResponse,
    );
    expect(resp.status).toBe(400);
  });
});
