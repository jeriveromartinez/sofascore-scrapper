import { describe, it, expect, beforeEach, vi } from "vitest";
import axios from "axios";
import { setActivePinia, createPinia } from "pinia";
import { writeAuthStorage, clearAuthStorage } from "../authStorage";
import { createAuthJsonAxios } from "./axiosAuth";

// The refreshAuth() helper decodes the protobuf AuthResponse with the real
// codec. We override `decode` to a JSON parser so we can return a known
// shape from the mocked axios.post call without dragging in the generated
// proto. Same pattern as BaseApiService.spec.ts.
vi.mock("../../proto/api", async () => {
  const actual =
    await vi.importActual<typeof import("../../proto/api")>(
      "../../proto/api",
    );
  return {
    ...actual,
    AuthResponse: {
      decode: (data: Uint8Array) => {
        const text = new TextDecoder().decode(data);
        return JSON.parse(text) as Record<string, unknown>;
      },
    },
  };
});

describe("createAuthJsonAxios", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    clearAuthStorage();
    vi.restoreAllMocks();
  });

  it("retries a failed request with the refreshed bearer token after a 401", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old-token", refreshToken: "old-refresh" },
      false,
    );
    setActivePinia(createPinia());

    const next = {
      id: 1,
      email: "a@b.com",
      token: "new-token",
      refreshToken: "new-refresh",
    };
    const body = new TextEncoder().encode(JSON.stringify(next)).buffer;
    vi.spyOn(axios, "post").mockResolvedValue({
      status: 200,
      data: body,
    });

    const instance = createAuthJsonAxios();
    const calls: Array<{ authHeader: string | undefined }> = [];
    instance.defaults.adapter = (config: {
      headers?: Record<string, unknown>;
    }) => {
      const raw = config.headers?.Authorization as string | undefined;
      calls.push({ authHeader: raw });
      if (calls.length === 1) {
        return Promise.reject({
          message: "Request failed with status code 401",
          name: "AxiosError",
          code: "ERR_BAD_REQUEST",
          response: {
            status: 401,
            statusText: "Unauthorized",
            data: {},
            headers: {},
            config,
          },
          config,
        } as never);
      }
      return Promise.resolve({
        status: 200,
        statusText: "OK",
        data: { ok: true },
        headers: {},
        config,
      } as never);
    };

    const res = await instance.get("/scraper-leagues", {
      headers: { Authorization: "Bearer old-token" },
    });
    expect(res.status).toBe(200);
    expect(res.data).toEqual({ ok: true });

    expect(calls).toHaveLength(2);
    expect(calls[0]?.authHeader).toBe("Bearer old-token");
    expect(calls[1]?.authHeader).toBe("Bearer new-token");
  });

  it("propagates 401 when the refresh token is rejected", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "bad-refresh" },
      false,
    );
    setActivePinia(createPinia());

    vi.spyOn(axios, "post").mockResolvedValue({
      status: 401,
      data: new ArrayBuffer(0),
    });

    const instance = createAuthJsonAxios();
    instance.defaults.adapter = (config: { headers?: unknown }) =>
      Promise.reject({
        message: "Request failed with status code 401",
        name: "AxiosError",
        code: "ERR_BAD_REQUEST",
        response: {
          status: 401,
          statusText: "Unauthorized",
          data: {},
          headers: {},
          config,
        },
        config,
      } as never);

    await expect(instance.get("/scraper-leagues")).rejects.toBeTruthy();
    // refresh endpoint was consulted (axios.post is the refresh transport).
    expect(axios.post).toHaveBeenCalledTimes(1);
  });

  it("does not retry login/register/refresh endpoints themselves", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "refresh" },
      false,
    );
    setActivePinia(createPinia());

    const postSpy = vi.spyOn(axios, "post");

    const instance = createAuthJsonAxios();
    let adapterCalls = 0;
    instance.defaults.adapter = (config: { url?: string; headers?: unknown }) => {
      adapterCalls += 1;
      return Promise.reject({
        message: "Request failed with status code 401",
        name: "AxiosError",
        code: "ERR_BAD_REQUEST",
        response: {
          status: 401,
          statusText: "Unauthorized",
          data: {},
          headers: {},
          config: { ...config, headers: {} },
        },
        config: { ...config, headers: {} },
      } as never);
    };

    await expect(instance.post("/users/login", {})).rejects.toBeTruthy();
    expect(adapterCalls).toBe(1);
    expect(postSpy).not.toHaveBeenCalled();
  });
});
