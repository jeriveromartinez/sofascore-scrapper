import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import axios from "axios";
import { setActivePinia, createPinia } from "pinia";
import { useAuthStore } from "../pinia/authStore";
import { writeAuthStorage, clearAuthStorage } from "../authStorage";
import { KEY_USER_LOGIN } from "../../constants";
import {
  headerString,
  ApiError,
  refreshAuth,
  BaseApiService,
  REQUEST_TIMEOUT_MS,
  UPLOAD_TIMEOUT_MS,
} from "./BaseApiService";
import { StatusMessage } from "../../proto/api";

// The refreshAuth() helper decodes the protobuf AuthResponse with the real codec.
// We override `decode` to a JSON parser so we can return a known AuthResponse
// shape from the mocked axios.post call without dragging in the generated proto.
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

describe("headerString", () => {
  it("returns the string value for a matching key", () => {
    expect(
      headerString({ "content-type": "application/x-protobuf" }, "content-type"),
    ).toBe("application/x-protobuf");
  });

  it("returns an empty string when the key is missing", () => {
    expect(headerString({}, "content-type")).toBe("");
    expect(headerString({ authorization: "Bearer x" }, "content-type")).toBe("");
  });

  it("falls back to an empty string for non-string header values", () => {
    expect(headerString({ "content-type": 42 }, "content-type")).toBe("");
    expect(headerString({ "content-type": true }, "content-type")).toBe("");
    expect(headerString({ "content-type": null }, "content-type")).toBe("");
    expect(headerString({ "content-type": ["a", "b"] }, "content-type")).toBe("");
  });

  it("returns an empty string when headers is null, undefined, or not an object", () => {
    expect(headerString(null, "content-type")).toBe("");
    expect(headerString(undefined, "content-type")).toBe("");
    expect(headerString("not-an-object", "content-type")).toBe("");
    expect(headerString(42, "content-type")).toBe("");
  });

  it("reads via .get() for an AxiosHeaders-like instance", () => {
    class FakeAxiosHeaders {
      private readonly map: Record<string, string>;
      constructor(values: Record<string, string>) {
        this.map = values;
      }
      get(name: string): string | undefined {
        return this.map[name.toLowerCase()];
      }
    }
    const headers = new FakeAxiosHeaders({
      "content-type": "application/json",
    });
    expect(headerString(headers, "content-type")).toBe("application/json");
  });

  it("reads via .get() case-insensitively for an AxiosHeaders-like instance", () => {
    class FakeAxiosHeaders {
      get(name: string): string | undefined {
        if (name.toLowerCase() === "content-type") {
          return "application/x-protobuf";
        }
        return undefined;
      }
    }
    const headers = new FakeAxiosHeaders();
    expect(headerString(headers, "content-type")).toBe(
      "application/x-protobuf",
    );
  });
});

describe("ApiError", () => {
  it("carries an isAuthError flag", () => {
    const err = new ApiError("auth failure", 401, true);
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("auth failure");
    expect(err.status).toBe(401);
    expect(err.isAuthError).toBe(true);
  });

  it("defaults isAuthError to false", () => {
    const err = new ApiError("boom", 500);
    expect(err.isAuthError).toBe(false);
  });
});

describe("refreshAuth", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    clearAuthStorage();
    vi.restoreAllMocks();
  });

  it("returns null when no refresh token is in storage", async () => {
    const postSpy = vi.spyOn(axios, "post");
    const result = await refreshAuth();
    expect(result).toBeNull();
    expect(postSpy).not.toHaveBeenCalled();
  });

  it("updates useAuthStore().userData with the new token on success", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "old-refresh" },
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

    const result = await refreshAuth();
    expect(result).not.toBeNull();
    expect(result?.token).toBe("new-token");

    const store = useAuthStore();
    expect(store.userData.token).toBe("new-token");
    expect(store.userData.refreshToken).toBe("new-refresh");
    expect(store.userData.email).toBe("a@b.com");
  });

  it("writes the new user to storage under KEY_USER_LOGIN (no literal)", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "old-refresh" },
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

    await refreshAuth();

    // The new user must be persisted under the canonical key in the
    // remembered storage (sessionStorage here, because rememberMe=false).
    const raw = sessionStorage.getItem(KEY_USER_LOGIN);
    expect(raw).not.toBeNull();
    const parsed = JSON.parse(raw as string);
    expect(parsed.token).toBe("new-token");
    expect(parsed.refreshToken).toBe("new-refresh");
  });

  it("persists to localStorage when rememberMe=true", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "old-refresh" },
      true,
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

    await refreshAuth();

    expect(localStorage.getItem(KEY_USER_LOGIN)).not.toBeNull();
    expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBeNull();
  });

  it("returns null when the refresh endpoint responds with non-200", async () => {
    writeAuthStorage(
      { id: 1, email: "a@b.com", token: "old", refreshToken: "old-refresh" },
      false,
    );
    setActivePinia(createPinia());

    vi.spyOn(axios, "post").mockResolvedValue({
      status: 401,
      data: new ArrayBuffer(0),
    });

    const result = await refreshAuth();
    expect(result).toBeNull();
    // Pinia must remain on the old user — the failed refresh must not
    // silently clear the store.
    const store = useAuthStore();
    expect(store.userData.token).toBe("old");
  });
});

describe("BaseApiService empty protobuf body", () => {
  class TestService extends BaseApiService {
    constructor() {
      super("/test");
    }

    fetchStatus(): Promise<StatusMessage> {
      return this.get("", StatusMessage);
    }
  }

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("decodes an empty body into the message defaults instead of undefined", async () => {
    const get = vi.fn(async () => ({
      data: new ArrayBuffer(0),
      status: 200,
      headers: {},
    }));
    vi.spyOn(axios, "create").mockReturnValue({
      get,
      interceptors: { response: { use: vi.fn() } },
    } as never);

    const result = await new TestService().fetchStatus();

    expect(result).toEqual({ message: "" });
    expect(result).not.toBeUndefined();
  });
});

describe("BaseApiService timeouts", () => {
  class UploadService extends BaseApiService {
    constructor() {
      super("/test");
    }

    upload(form: FormData): Promise<StatusMessage> {
      return this.postMultipart("", form, StatusMessage);
    }

    uploadJSON(form: FormData): Promise<unknown> {
      return this.postMultipartJSON("", form);
    }
  }

  beforeEach(() => {
    setActivePinia(createPinia());
    clearAuthStorage();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("creates its axios instance with a finite request timeout", () => {
    const create = vi.spyOn(axios, "create");
    new UploadService();
    expect(REQUEST_TIMEOUT_MS).toBeGreaterThan(0);
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ timeout: REQUEST_TIMEOUT_MS }));
  });

  it("gives multipart uploads a longer explicit timeout", async () => {
    const post = vi.fn(async () => ({ data: new ArrayBuffer(0), status: 200, headers: {} }));
    vi.spyOn(axios, "create").mockReturnValue({
      post,
      interceptors: { response: { use: vi.fn() } },
    } as never);
    const service = new UploadService();

    await service.upload(new FormData());
    await service.uploadJSON(new FormData());

    expect(UPLOAD_TIMEOUT_MS).toBeGreaterThan(REQUEST_TIMEOUT_MS);
    for (const call of post.mock.calls as unknown as Array<[string, unknown, { timeout?: number }]>) {
      expect(call[2].timeout).toBe(UPLOAD_TIMEOUT_MS);
    }
    expect(post).toHaveBeenCalledTimes(2);
  });

  it("bounds the refresh request with the request timeout", async () => {
    writeAuthStorage({ id: 1, email: "a@b.com", token: "t", refreshToken: "r" }, false);
    const post = vi.spyOn(axios, "post").mockResolvedValue({ status: 401, data: new ArrayBuffer(0) });

    await refreshAuth();

    expect(post).toHaveBeenCalledWith(
      expect.any(String),
      undefined,
      expect.objectContaining({ timeout: REQUEST_TIMEOUT_MS }),
    );
  });

  it("starts a new refresh after a timed-out one instead of reusing it", async () => {
    writeAuthStorage({ id: 1, email: "a@b.com", token: "t", refreshToken: "r" }, false);
    const post = vi
      .spyOn(axios, "post")
      .mockRejectedValueOnce(Object.assign(new Error("timeout of 30000ms exceeded"), { code: "ECONNABORTED" }))
      .mockResolvedValueOnce({ status: 401, data: new ArrayBuffer(0) });

    await expect(refreshAuth()).rejects.toThrow("timeout");
    await expect(refreshAuth()).resolves.toBeNull();

    expect(post).toHaveBeenCalledTimes(2);
  });
});

describe("BaseApiService cancellation", () => {
  class AllVerbsService extends BaseApiService {
    constructor() {
      super("/test");
    }

    calls(signal: AbortSignal): Array<Promise<unknown>> {
      const form = new FormData();
      return [
        this.post("", { message: "x" }, StatusMessage, StatusMessage, signal),
        this.postWithoutBody("", StatusMessage, undefined, signal),
        this.put("", { message: "x" }, StatusMessage, StatusMessage, signal),
        this.patch("", { message: "x" }, StatusMessage, StatusMessage, signal),
        this.postMultipart("", form, StatusMessage, signal),
        this.postMultipartJSON("", form, signal),
        this.getBinary("", signal),
        this.delete("", StatusMessage, signal),
        this.deleteWithBody("", { message: "x" }, StatusMessage, StatusMessage, signal),
      ];
    }
  }

  beforeEach(() => {
    setActivePinia(createPinia());
    clearAuthStorage();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("forwards the AbortSignal on every verb", async () => {
    const configs: Array<{ signal?: AbortSignal }> = [];
    const respond = async () => ({ data: new ArrayBuffer(0), status: 200, headers: {} });
    const withBody = vi.fn(async (_url: string, _body: unknown, config: { signal?: AbortSignal }) => {
      configs.push(config);
      return respond();
    });
    const noBody = vi.fn(async (_url: string, config: { signal?: AbortSignal }) => {
      configs.push(config);
      return respond();
    });
    vi.spyOn(axios, "create").mockReturnValue({
      post: withBody,
      put: withBody,
      patch: withBody,
      get: noBody,
      delete: noBody,
      interceptors: { response: { use: vi.fn() } },
    } as never);
    const controller = new AbortController();

    await Promise.all(new AllVerbsService().calls(controller.signal));

    expect(configs).toHaveLength(9);
    for (const config of configs) {
      expect(config.signal).toBe(controller.signal);
    }
  });
});
