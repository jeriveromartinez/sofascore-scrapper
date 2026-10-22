import { afterEach, describe, expect, it, vi } from "vitest";
import axios from "axios";
import { ApkApiService } from "./ApkApiService";
import { APP_API_BASE_URL } from "../../constants";
import { UPLOAD_TIMEOUT_MS } from "./BaseApiService";

describe("ApkApiService client-facing endpoints", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("checkUpdate queries the app update endpoint", async () => {
    const get = vi.spyOn(axios, "get").mockResolvedValue({
      status: 200,
      data: new ArrayBuffer(0),
      headers: {},
    });

    const svc = new ApkApiService();
    await svc.checkUpdate("1.0.0", "com.example.app");

    const url = get.mock.calls[0]?.[0] as string;
    expect(url.startsWith(`${APP_API_BASE_URL}/update?`)).toBe(true);
    expect(url).toContain("version=1.0.0");
    expect(url).toContain("package=com.example.app");
    expect(url).not.toContain("/apk/check");
  });

  it("checkUpdate throws the backend error message on a failed request", async () => {
    const body = new TextEncoder().encode(JSON.stringify({ error: "no APK version available" }));
    vi.spyOn(axios, "get").mockResolvedValue({
      status: 404,
      data: body.buffer as ArrayBuffer,
      headers: {},
    });

    const svc = new ApkApiService();
    await expect(svc.checkUpdate("1.0.0", "com.example.app")).rejects.toThrow(
      "no APK version available",
    );
  });

  it("downloadByToken fetches the APK from the app API", async () => {
    const get = vi.spyOn(axios, "get").mockResolvedValue({
      status: 200,
      data: new ArrayBuffer(4),
      headers: {},
    });

    const svc = new ApkApiService();
    const blob = await svc.downloadByToken("tok-123");

    const url = get.mock.calls[0]?.[0] as string;
    expect(url).toBe(`${APP_API_BASE_URL}/apk/download/tok-123`);
    expect(blob.size).toBe(4);
  });
});

describe("ApkApiService chunk upload", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends chunks with the upload timeout", async () => {
    const put = vi.spyOn(axios, "put").mockResolvedValue({ status: 200, data: new ArrayBuffer(0) });

    await new ApkApiService().putChunk("up-1", 0, new Blob([new Uint8Array([1, 2, 3])]));

    expect(put).toHaveBeenCalledWith(
      expect.stringContaining("/apk/uploads/up-1/chunks/0"),
      expect.anything(),
      expect.objectContaining({ timeout: UPLOAD_TIMEOUT_MS }),
    );
  });
});
