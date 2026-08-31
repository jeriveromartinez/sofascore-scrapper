import { describe, it, expect, beforeEach, vi } from "vitest";
import axios from "axios";
import { setActivePinia, createPinia } from "pinia";
import { useAuthStore } from "../pinia/authStore";
import { writeAuthStorage, clearAuthStorage } from "../authStorage";
import { KEY_USER_LOGIN } from "../../constants";
import { headerString, ApiError, refreshAuth } from "./BaseApiService";

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
