import { afterEach, describe, expect, it, vi } from "vitest";

describe("API_BASE_URL", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("defaults to the same-origin web API", async () => {
    vi.stubEnv("VITE_API_BASE_URL", undefined);
    const { API_BASE_URL } = await import("./constants");

    expect(API_BASE_URL).toBe("/api/web/v1");
  });
});
