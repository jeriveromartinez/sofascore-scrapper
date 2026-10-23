import { request as playwrightRequest, type APIRequestContext } from "@playwright/test";
import { randomBytes } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import {
  api,
  AuthRequest,
  AuthResponse,
  E2E_ADMIN_EMAIL,
  E2E_ADMIN_PASSWORD,
  E2E_CLIENT_IP_PATH,
  E2E_SEEDED_ADMIN_EMAIL,
  E2E_SEEDED_ADMIN_PASSWORD,
  E2E_STATE_PATH,
  E2E_USER_EMAIL,
  E2E_USER_PASSWORD,
  StatusMessage,
  type E2EState,
} from "./helpers";
import { SetUserRoleRequest, User, UserList, UserWriteRequest } from "../src/proto/api";

const AUTH_PATH = "/api/web/v1/users";
const UPLOADS_PATH = "/api/web/v1/apk/uploads";

// The auth endpoints are throttled per client address. The suite runs
// from the host, so all of its requests would arrive from the same
// address and a run would throttle the next one. Every run therefore
// acts as a client of its own, the way a different device behind the
// proxy of a deployment would.
function writeClientAddress(): void {
  const bytes = randomBytes(3);
  writeFileSync(E2E_CLIENT_IP_PATH, `10.${bytes[0]}.${bytes[1]}.${bytes[2] || 1}`, {
    mode: 0o600,
  });
}

function readCachedState(): E2EState | null {
  try {
    if (!existsSync(E2E_STATE_PATH)) return null;
    const parsed = JSON.parse(readFileSync(E2E_STATE_PATH, "utf-8")) as E2EState;
    return { ...parsed, openUploadIds: parsed.openUploadIds ?? [] };
  } catch {
    return null;
  }
}

async function login(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const resp = await api.post<AuthResponse, AuthRequest>(
    request,
    `${AUTH_PATH}/login`,
    { email, password, invitationToken: "" },
    AuthRequest,
    AuthResponse,
  );
  return resp.status === 200 && resp.data?.token ? resp.data.token : "";
}

async function findUser(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<User | null> {
  const resp = await api.get<UserList>(request, AUTH_PATH, UserList, {
    Authorization: `Bearer ${token}`,
  });
  if (resp.status !== 200) return null;
  return resp.data?.users.find((u) => u.email === email) ?? null;
}

async function createUser(
  request: APIRequestContext,
  adminToken: string,
  email: string,
  password: string,
): Promise<number> {
  const resp = await api.post<User, UserWriteRequest>(
    request,
    AUTH_PATH,
    { email, password },
    UserWriteRequest,
    User,
    { Authorization: `Bearer ${adminToken}` },
  );
  if (resp.status === 201 && resp.data) return resp.data.id;

  // The account exists already, so the conflict is the expected
  // outcome of a second run and the id is looked up instead.
  const existing = await findUser(request, adminToken, email);
  return existing?.id ?? 0;
}

async function setRole(
  request: APIRequestContext,
  adminToken: string,
  userId: number,
  role: string,
): Promise<void> {
  const resp = await api.put<User, SetUserRoleRequest>(
    request,
    `${AUTH_PATH}/${userId}/role`,
    { role },
    SetUserRoleRequest,
    User,
    { Authorization: `Bearer ${adminToken}` },
  );
  if (resp.status !== 200) {
    throw new Error(`could not set the role of user ${userId}: ${resp.error ?? resp.status}`);
  }
}

// The backend counts the upload sessions that are still open for a
// user and refuses to open more than a couple of them. A session
// counts for 24 hours, so a run that was interrupted leaves the next
// one unable to open one. The suite writes the id of every session it
// opens into its state file, and the setup of the next run aborts them
// through the API.
async function abortLeftoverUploads(
  request: APIRequestContext,
  adminToken: string,
  uploadIds: string[],
): Promise<void> {
  for (const uploadId of uploadIds) {
    const resp = await api.delete<StatusMessage>(
      request,
      `${UPLOADS_PATH}/${uploadId}`,
      StatusMessage,
      { Authorization: `Bearer ${adminToken}` },
    );
    if (resp.status !== 200) {
      console.warn(`could not abort the upload session ${uploadId}: ${resp.error ?? resp.status}`);
    }
  }
}

// The suite administers the stack with accounts of its own. They are
// created once and reused by every run, so a run never depends on the
// rows an earlier run left behind.
async function ensureAccounts(request: APIRequestContext): Promise<E2EState> {
  const cached = readCachedState();
  if (cached?.adminToken) {
    const admin = await findUser(request, cached.adminToken, E2E_ADMIN_EMAIL);
    if (admin) {
      // A run that failed while promoting the shared account leaves it
      // as an administrator, which would break the role assertions.
      if (cached.userId > 0) {
        await setRole(request, cached.adminToken, cached.userId, "user");
      }
      return {
        adminToken: cached.adminToken,
        adminUserId: admin.id,
        userId: cached.userId,
        openUploadIds: cached.openUploadIds,
      };
    }
  }

  let adminToken = await login(request, E2E_ADMIN_EMAIL, E2E_ADMIN_PASSWORD);
  let adminUserId = 0;

  if (adminToken) {
    adminUserId = (await findUser(request, adminToken, E2E_ADMIN_EMAIL))?.id ?? 0;
  } else {
    // The account does not exist yet, so the seeded administrator
    // creates it and promotes it.
    const seededToken = await login(
      request,
      E2E_SEEDED_ADMIN_EMAIL,
      E2E_SEEDED_ADMIN_PASSWORD,
    );
    if (!seededToken) {
      throw new Error(
        `Could not log in as ${E2E_SEEDED_ADMIN_EMAIL}. The stack seeds that account on first boot; ` +
          "set E2E_SEEDED_ADMIN_EMAIL and E2E_SEEDED_ADMIN_PASSWORD for a different target.",
      );
    }
    adminUserId = await createUser(
      request,
      seededToken,
      E2E_ADMIN_EMAIL,
      E2E_ADMIN_PASSWORD,
    );
    if (!adminUserId) throw new Error("could not create the e2e administrator account");
    await setRole(request, seededToken, adminUserId, "admin");
    adminToken = await login(request, E2E_ADMIN_EMAIL, E2E_ADMIN_PASSWORD);
    if (!adminToken) throw new Error("could not log in as the e2e administrator");
  }

  const userId = await createUser(
    request,
    adminToken,
    E2E_USER_EMAIL,
    E2E_USER_PASSWORD,
  );
  if (!userId) throw new Error("could not create the e2e user account");

  // A run that failed while promoting the account leaves it as an
  // administrator, which would break the role assertions of the suite.
  await setRole(request, adminToken, userId, "user");

  return { adminToken, adminUserId, userId, openUploadIds: [] };
}

export default async function globalSetup(): Promise<void> {
  writeClientAddress();

  const request = await playwrightRequest.newContext({
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:8080",
  });

  try {
    const state = await ensureAccounts(request);
    await abortLeftoverUploads(request, state.adminToken, state.openUploadIds);
    writeFileSync(E2E_STATE_PATH, JSON.stringify({ ...state, openUploadIds: [] }), { mode: 0o600 });
  } finally {
    await request.dispose();
  }
}
