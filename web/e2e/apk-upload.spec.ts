import { test, expect } from "@playwright/test";
import { api, AuthRequest, AuthResponse, UploadBeginRequest, UploadBeginResponse, UploadStatusResponse, UploadChunkResponse, StatusMessage, DeviceUrl } from "./helpers";

const AUTH_PATH = "/api/web/v1/users";
const UPLOADS_PATH = "/api/web/v1/apk/uploads";
const APK_PATH = "/api/web/v1/apk";

const TEST_EMAIL = `e2e-apk-${Date.now()}@test.local`;
const TEST_PASSWORD = "Password1!";

let accessToken: string;

test.describe("Resumable APK upload", () => {
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

  test("begin upload requires auth", async ({ request }) => {
    const resp = await api.post<UploadBeginResponse, UploadBeginRequest>(
      request,
      UPLOADS_PATH,
      { fileName: "test.apk", fileSize: 1024, totalChunks: 1, version: "1.0.0", description: "" },
      UploadBeginRequest,
      UploadBeginResponse,
    );
    expect(resp.status).toBe(401);
  });

  test("begin upload with invalid params fails", async ({ request }) => {
    // totalChunks=0 is invalid
    const resp = await api.post<UploadBeginResponse, UploadBeginRequest>(
      request,
      UPLOADS_PATH,
      { fileName: "test.apk", fileSize: 0, totalChunks: 0, version: "1.0.0", description: "" },
      UploadBeginRequest,
      UploadBeginResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(409);
  });

  test("full resumable upload lifecycle: begin → status → put chunk → abort", async ({ request }) => {
    const DUMMY_SIZE = 512;
    const TOTAL_CHUNKS = 1;

    // Step 1: Begin upload
    const beginResp = await api.post<UploadBeginResponse, UploadBeginRequest>(
      request,
      UPLOADS_PATH,
      { fileName: "dummy.apk", fileSize: DUMMY_SIZE, totalChunks: TOTAL_CHUNKS, version: "9.9.9", description: "test" },
      UploadBeginRequest,
      UploadBeginResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(beginResp.status, `begin error: ${beginResp.error}`).toBe(201);
    const uploadId = beginResp.data!.uploadId;
    expect(uploadId).toBeTruthy();
    expect(beginResp.data!.status).toBe("receiving");

    // Step 2: Check status
    const statusResp = await api.get<UploadStatusResponse>(
      request,
      `${UPLOADS_PATH}/${uploadId}`,
      UploadStatusResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(statusResp.status, `status error: ${statusResp.error}`).toBe(200);
    expect(statusResp.data!.status).toBe("receiving");

    // Step 3: Upload chunk 0 (raw bytes)
    const chunkData = Buffer.alloc(DUMMY_SIZE, 0xAB);
    const chunkResp = await api.putRaw(
      request,
      `${UPLOADS_PATH}/${uploadId}/chunks/0`,
      new Uint8Array(chunkData),
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(chunkResp.status, `chunk error: ${chunkResp.error}`).toBe(200);

    // Step 4: Abort
    const abortResp = await api.delete<StatusMessage>(
      request,
      `${UPLOADS_PATH}/${uploadId}`,
      StatusMessage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(abortResp.status, `abort error: ${abortResp.error}`).toBe(200);
  });

  test("non-existent upload returns 404", async ({ request }) => {
    const resp = await api.get<UploadStatusResponse>(
      request,
      `${UPLOADS_PATH}/00000000-0000-0000-0000-000000000000`,
      UploadStatusResponse,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(404);
  });

  test("APK URL edit requires auth", async ({ request }) => {
    const resp = await api.put<StatusMessage, DeviceUrl>(
      request,
      `${APK_PATH}/1`,
      { url: "https://example.com/panel" },
      DeviceUrl,
      StatusMessage,
    );
    expect(resp.status).toBe(401);
  });

  test("APK URL edit returns 404 for non-existent APK", async ({ request }) => {
    const resp = await api.put<StatusMessage, DeviceUrl>(
      request,
      `${APK_PATH}/99999`,
      { url: "https://example.com/panel" },
      DeviceUrl,
      StatusMessage,
      { Authorization: `Bearer ${accessToken}` },
    );
    expect(resp.status).toBe(404);
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
