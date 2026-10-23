import type { APIRequestContext } from "@playwright/test";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  AuthRequest,
  AuthResponse,
  TournamentRequest,
  Tournament,
  TournamentList,
  TournamentPage,
  StatusMessage,
  UploadBeginRequest,
  UploadBeginResponse,
  UploadStatusResponse,
  UploadCompleteResponse,
  UploadChunkResponse,
  CreateInvitationRequest,
  InvitationResponse,
  DeviceUrl,
  EventsList,
} from "../src/proto/api";

const PROTO_CONTENT_TYPE = "application/x-protobuf";
const AUTH_PATH = "/api/web/v1/users";

export const E2E_STATE_PATH = join(tmpdir(), "scores365-e2e-state.json");
export const E2E_CLIENT_IP_PATH = join(tmpdir(), "scores365-e2e-client-ip");

// The API rejects registrations whose password is shorter than 12
// characters, so the fixtures below have to satisfy that policy.
//
// The accounts are created once by the global setup and reused by
// every run. A dedicated administrator keeps the upload sessions of
// the suite away from the quota of the seeded account, whose history
// on the stack the suite cannot know.
export const E2E_ADMIN_EMAIL = "e2e-admin@test.local";
export const E2E_ADMIN_PASSWORD = "E2eAdminPass1!";
export const E2E_USER_EMAIL = "e2e-user@test.local";
export const E2E_USER_PASSWORD = "E2eUserPass1!";

// The seeded administrator (internal/seeder) is the only account with
// admin rights on a fresh stack, so the suite logs in as that one. The
// password comes from DEFAULT_ADMIN_PASSWORD, which the test stack sets;
// E2E_ADMIN_EMAIL / E2E_ADMIN_PASSWORD override it for other targets.
export const E2E_SEEDED_ADMIN_EMAIL =
  process.env.E2E_SEEDED_ADMIN_EMAIL ?? "admin@local";
export const E2E_SEEDED_ADMIN_PASSWORD =
  process.env.E2E_SEEDED_ADMIN_PASSWORD ?? "E2eStackAdminPass1!";

// The state the global setup hands to the specs: the tokens and the
// ids of the accounts the suite administers the stack with.
//
// The open upload sessions are part of it as well. A session counts
// against the quota of its user for 24 hours, so a run that was
// interrupted leaves the next one unable to open a session. The ids
// are written down as soon as the session exists, and the next run
// aborts them through the API.
export interface E2EState {
  adminToken: string;
  adminUserId: number;
  userId: number;
  openUploadIds: string[];
}

let cachedState: E2EState | null = null;

interface ProtoCodec<T> {
  encode(message: T): { finish(): Uint8Array };
  decode(input: Uint8Array): T;
}

// The stack trusts the proxy in front of it, so the requests of the
// suite carry the address of the client they belong to. The auth
// endpoints are throttled per client address, and without it the suite
// would throttle itself with the traffic of the previous run.
export function getE2EClientIp(): string {
  if (!existsSync(E2E_CLIENT_IP_PATH)) {
    throw new Error(
      `E2E client address file ${E2E_CLIENT_IP_PATH} is missing; the global setup did not run`,
    );
  }
  return readFileSync(E2E_CLIENT_IP_PATH, "utf-8").trim();
}

function clientHeaders(headers: Record<string, string>): Record<string, string> {
  return { "X-Forwarded-For": getE2EClientIp(), ...headers };
}

async function protoPost<TRes, TReq>(
  request: APIRequestContext,
  url: string,
  body: TReq,
  encoder: ProtoCodec<TReq>,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  });
  const payload = encoder.encode(body).finish();
  const resp = await request.post(url, {
    headers: reqHeaders,
    data: Buffer.from(payload),
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  return parseResponse(resp.status(), respBody, resp.headers()["content-type"] ?? "", decoder);
}

async function protoPostNoBody<TRes>(
  request: APIRequestContext,
  url: string,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  });
  const resp = await request.post(url, {
    headers: reqHeaders,
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  return parseResponse(resp.status(), respBody, resp.headers()["content-type"] ?? "", decoder);
}

async function protoPut<TRes, TReq>(
  request: APIRequestContext,
  url: string,
  body: TReq,
  encoder: ProtoCodec<TReq>,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  });
  const payload = encoder.encode(body).finish();
  const resp = await request.put(url, {
    headers: reqHeaders,
    data: Buffer.from(payload),
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  return parseResponse(resp.status(), respBody, resp.headers()["content-type"] ?? "", decoder);
}

async function protoPutRaw(
  request: APIRequestContext,
  url: string,
  body: Uint8Array,
  headers: Record<string, string> = {},
): Promise<{ status: number; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  });
  const resp = await request.put(url, {
    headers: reqHeaders,
    data: Buffer.from(body),
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  const err = parseJsonError(respBody);
  return { status: resp.status(), error: err };
}

async function protoGet<TRes>(
  request: APIRequestContext,
  url: string,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  });
  const resp = await request.get(url, {
    headers: reqHeaders,
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  return parseResponse(resp.status(), respBody, resp.headers()["content-type"] ?? "", decoder);
}

async function protoDelete<TRes>(
  request: APIRequestContext,
  url: string,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders = clientHeaders({
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  });
  const resp = await request.delete(url, {
    headers: reqHeaders,
    failOnStatusCode: false,
  });
  const respBody = await resp.body();
  return parseResponse(resp.status(), respBody, resp.headers()["content-type"] ?? "", decoder);
}

function parseResponse<T>(
  status: number,
  body: Buffer,
  contentType: string,
  decoder: ProtoCodec<T>,
): { status: number; data: T | null; error: string | null } {
  if (!body.byteLength) {
    if (status >= 400) return { status, data: null, error: `HTTP ${status}` };
    if (contentType.includes(PROTO_CONTENT_TYPE)) {
      return { status, data: decoder.decode(new Uint8Array()), error: null };
    }
    return { status, data: null, error: null };
  }
  if (status >= 400) {
    const jsonErr = parseJsonError(body);
    if (jsonErr) return { status, data: null, error: jsonErr };
    return { status, data: null, error: `HTTP ${status}` };
  }
  if (contentType.includes(PROTO_CONTENT_TYPE)) {
    return { status, data: decoder.decode(new Uint8Array(body)), error: null };
  }
  return { status, data: null, error: null };
}

function parseJsonError(body: Buffer): string | null {
  try {
    const text = new TextDecoder().decode(body);
    const parsed = JSON.parse(text) as { error?: string };
    return parsed.error ?? null;
  } catch {
    return null;
  }
}

export async function createTestInvitation(request: APIRequestContext): Promise<string> {
  const accessToken = getE2EAdminToken();
  const response = await protoPost<InvitationResponse, CreateInvitationRequest>(
    request,
    `${AUTH_PATH}/invitations`,
    { ttlSeconds: 600 },
    CreateInvitationRequest,
    InvitationResponse,
    { Authorization: `Bearer ${accessToken}` },
  );
  if (response.status !== 201 || !response.data?.token) {
    throw new Error(`Could not create E2E invitation: ${response.error ?? response.status}`);
  }
  return response.data.token;
}

export function getE2EState(): E2EState {
  if (cachedState) return cachedState;
  if (!existsSync(E2E_STATE_PATH)) {
    throw new Error(`E2E state file ${E2E_STATE_PATH} is missing; the global setup did not run`);
  }
  const parsed = JSON.parse(readFileSync(E2E_STATE_PATH, "utf-8")) as E2EState;
  cachedState = { ...parsed, openUploadIds: parsed.openUploadIds ?? [] };
  return cachedState;
}

function writeE2EState(state: E2EState): void {
  cachedState = state;
  writeFileSync(E2E_STATE_PATH, JSON.stringify(state), { mode: 0o600 });
}

// The upload session of a spec is written down the moment it exists, so
// a run that is killed before it can release the session still leaves
// the next run able to clean it up.
export function trackOpenUpload(uploadId: string): void {
  const state = getE2EState();
  if (state.openUploadIds.includes(uploadId)) return;
  writeE2EState({ ...state, openUploadIds: [...state.openUploadIds, uploadId] });
}

export function releaseOpenUpload(uploadId: string): void {
  const state = getE2EState();
  if (!state.openUploadIds.includes(uploadId)) return;
  writeE2EState({
    ...state,
    openUploadIds: state.openUploadIds.filter((id) => id !== uploadId),
  });
}

export function getE2EAdminToken(): string {
  return getE2EState().adminToken;
}

export const api = {
  post: protoPost,
  postNoBody: protoPostNoBody,
  put: protoPut,
  putRaw: protoPutRaw,
  get: protoGet,
  delete: protoDelete,
};

export { AuthRequest, AuthResponse, TournamentRequest, Tournament, TournamentList, TournamentPage, StatusMessage, UploadBeginRequest, UploadBeginResponse, UploadStatusResponse, UploadCompleteResponse, UploadChunkResponse, CreateInvitationRequest, InvitationResponse, DeviceUrl, EventsList };
