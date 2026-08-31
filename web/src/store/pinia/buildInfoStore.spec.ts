import { describe, it, expect, beforeEach, vi } from "vitest";
import { setActivePinia, createPinia } from "pinia";
import { useBuildInfoStore } from "./buildInfoStore";
import { BuildInfo } from "../../proto/api";

vi.mock("axios", () => ({
  create: vi.fn().mockReturnValue({
    interceptors: { response: { use: vi.fn() } },
  }),
}));

vi.mock("../services/BuildInfoService", () => {
  const getBuildInfo = vi.fn();
  class MockBuildInfoService {
    constructor() {
      Object.defineProperty(this, "getBuildInfo", {
        value: getBuildInfo,
        writable: true,
        configurable: true,
      });
    }
  }
  return { BuildInfoService: MockBuildInfoService, getBuildInfo };
});

import { BuildInfoService } from "../services/BuildInfoService";

describe("useBuildInfoStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("starts empty", () => {
    const store = useBuildInfoStore();
    expect(store.version).toBe("");
    expect(store.commit).toBe("");
    expect(store.loaded).toBe(false);
  });

  it("load() fetches and populates", async () => {
    const getBuildInfo = vi.mocked(new BuildInfoService().getBuildInfo);
    getBuildInfo.mockResolvedValue(
      BuildInfo.create({ version: "v0.0.4", commit: "a0db9ad" }),
    );
    const store = useBuildInfoStore();
    await store.load();
    expect(store.version).toBe("v0.0.4");
    expect(store.commit).toBe("a0db9ad");
    expect(store.loaded).toBe(true);
  });

  it("load() is idempotent within a session", async () => {
    const getBuildInfo = vi.mocked(new BuildInfoService().getBuildInfo);
    getBuildInfo.mockResolvedValue(
      BuildInfo.create({ version: "v0.0.4", commit: "a0db9ad" }),
    );
    const store = useBuildInfoStore();
    await store.load();
    await store.load();
    expect(getBuildInfo).toHaveBeenCalledTimes(1);
  });

  it("load() swallows errors (no version shown is acceptable)", async () => {
    const getBuildInfo = vi.mocked(new BuildInfoService().getBuildInfo);
    getBuildInfo.mockRejectedValue(new Error("network down"));
    const store = useBuildInfoStore();
    await expect(store.load()).resolves.toBeUndefined();
    expect(store.version).toBe("");
    expect(store.commit).toBe("");
    expect(store.loaded).toBe(false);
  });
});
