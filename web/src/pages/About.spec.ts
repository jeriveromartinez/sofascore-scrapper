import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import { useBuildInfoStore } from "../store/pinia/buildInfoStore";
import { BuildInfo } from "../proto/api";
import About from "./About.vue";

vi.mock("../store/services/BuildInfoService", () => {
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

import { BuildInfoService, getBuildInfo as mockGetBuildInfo } from "../store/services/BuildInfoService";

describe("About.vue", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("shows the build version", async () => {
    mockGetBuildInfo.mockResolvedValue(
      BuildInfo.create({ version: "v0.0.4", commit: "a0db9ad" }),
    );
    const wrapper = mount(About);
    const store = useBuildInfoStore();
    await store.load();
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("v0.0.4");
    expect(wrapper.text()).toContain("a0db9ad");
  });

  it("shows a loading hint when the store is not loaded", async () => {
    mockGetBuildInfo.mockRejectedValue(new Error("network down"));
    const wrapper = mount(About);
    const store = useBuildInfoStore();
    await store.load();
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("Loading");
  });
});
