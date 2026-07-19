import { test, expect } from "@playwright/test";
import { api, getE2EAdminToken, CreateInvitationRequest, InvitationResponse, UploadBeginRequest, UploadBeginResponse } from "./helpers";

const INVITE_PATH = "/api/web/v1/users/invitations";
const UPLOADS_PATH = "/api/web/v1/apk/uploads";

let accessToken: string;

const COMPOSE_FILE = "../deployments/docker/compose.test.yml";

async function runDocker(args: string): Promise<void> {
  const { execSync } = await import("child_process");
  try {
    execSync(`docker compose -f ${COMPOSE_FILE} ${args}`, {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 30000,
    });
  } catch (e: any) {
    console.warn(`docker warning: ${e.message}`);
  }
}

async function pauseRedis(): Promise<void> {
  await runDocker("pause redis");
  // Give time for TCP connections to time out
  await new Promise((r) => setTimeout(r, 2000));
}

async function unpauseRedis(): Promise<void> {
  await runDocker("unpause redis");
  await new Promise((r) => setTimeout(r, 1000));
}

test.describe("Redis degradation", () => {
  test.beforeAll(() => {
    accessToken = getE2EAdminToken();
  });

  test("with Redis healthy: invitation creation works", async ({ request }) => {
    const resp = await api.post<InvitationResponse, CreateInvitationRequest>(
      request,
      INVITE_PATH,
      { ttlSeconds: 300 },
      CreateInvitationRequest,
      InvitationResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status, `invite error: ${resp.error}`).toBe(201);
  });

  test("with Redis healthy: upload begin works", async ({ request }) => {
    const resp = await api.post<UploadBeginResponse, UploadBeginRequest>(
      request,
      UPLOADS_PATH,
      { fileName: "test.apk", fileSize: 100, totalChunks: 1, version: "1.0.0", description: "" },
      UploadBeginRequest,
      UploadBeginResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status, `upload error: ${resp.error}`).toBe(201);
  });

  test("with Redis paused: invitation creation returns 503 (admin routes fail-closed)", async ({ request }) => {
    await pauseRedis();
    try {
      const resp = await api.post<InvitationResponse, CreateInvitationRequest>(
        request,
        INVITE_PATH,
        { ttlSeconds: 300 },
        CreateInvitationRequest,
        InvitationResponse,
        { Authorization: `Bearer ${accessToken}` },
      );
      expect(resp.status).toBe(503);
    } finally {
      await unpauseRedis();
    }
  });

  test("with Redis paused: upload begin returns 503 (admin routes fail-closed)", async ({ request }) => {
    await pauseRedis();
    try {
      const resp = await api.post<UploadBeginResponse, UploadBeginRequest>(
        request,
        UPLOADS_PATH,
        { fileName: "test.apk", fileSize: 100, totalChunks: 1, version: "1.0.0", description: "" },
        UploadBeginRequest,
        UploadBeginResponse,
        { Authorization: `Bearer ${accessToken}` },
      );
      expect(resp.status).toBe(503);
    } finally {
      await unpauseRedis();
    }
  });

  test("after Redis recovery: invitation works again", async ({ request }) => {
    const resp = await api.post<InvitationResponse, CreateInvitationRequest>(
      request,
      INVITE_PATH,
      { ttlSeconds: 300 },
      CreateInvitationRequest,
      InvitationResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(201);
  });

  test("after Redis recovery: upload begin works again", async ({ request }) => {
    const resp = await api.post<UploadBeginResponse, UploadBeginRequest>(
      request,
      UPLOADS_PATH,
      { fileName: "test.apk", fileSize: 100, totalChunks: 1, version: "1.0.0", description: "" },
      UploadBeginRequest,
      UploadBeginResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(201);
  });
});
