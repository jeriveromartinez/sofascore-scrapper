import type { APIRequestContext } from "@playwright/test";
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

interface ProtoCodec<T> {
  encode(message: T): { finish(): Uint8Array };
  decode(input: Uint8Array): T;
}

async function protoPost<TRes, TReq>(
  request: APIRequestContext,
  url: string,
  body: TReq,
  encoder: ProtoCodec<TReq>,
  decoder: ProtoCodec<TRes>,
  headers: Record<string, string> = {},
): Promise<{ status: number; data: TRes | null; error: string | null }> {
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  };
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
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  };
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
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  };
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
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    "Content-Type": PROTO_CONTENT_TYPE,
    ...headers,
  };
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
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  };
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
  const reqHeaders: Record<string, string> = {
    Accept: PROTO_CONTENT_TYPE,
    ...headers,
  };
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

export const api = {
  post: protoPost,
  postNoBody: protoPostNoBody,
  put: protoPut,
  putRaw: protoPutRaw,
  get: protoGet,
  delete: protoDelete,
};

export { AuthRequest, AuthResponse, TournamentRequest, Tournament, TournamentList, TournamentPage, StatusMessage, UploadBeginRequest, UploadBeginResponse, UploadStatusResponse, UploadCompleteResponse, UploadChunkResponse, CreateInvitationRequest, InvitationResponse, DeviceUrl, EventsList };
