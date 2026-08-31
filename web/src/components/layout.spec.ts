import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import { useBuildInfoStore } from "../store/pinia/buildInfoStore";
import { BuildInfo } from "../proto/api";
import Layout from "./layout.vue";

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

import { BuildInfoService } from "../store/services/BuildInfoService";

function mountLayout(): ReturnType<typeof mount> {
  return mount(Layout, {
    global: {
      mocks: { $router: { push: vi.fn() } },
      stubs: ["router-view", "left-nav-bar", "theme-selector"],
    },
  });
}

describe("layout.vue footer", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("shows the build version in the footer when loaded", async () => {
    const getBuildInfo = vi.mocked(new BuildInfoService().getBuildInfo);
    getBuildInfo.mockResolvedValue(
      BuildInfo.create({ version: "v0.0.4", commit: "a0db9ad" }),
    );
    const wrapper = mountLayout();
    const store = useBuildInfoStore();
    await store.load();
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("v0.0.4");
    expect(wrapper.text()).toContain("a0db9ad");
  });

  it("does not show the version when the store is not loaded", async () => {
    const getBuildInfo = vi.mocked(new BuildInfoService().getBuildInfo);
    getBuildInfo.mockRejectedValue(new Error("network down"));
    const wrapper = mountLayout();
    const store = useBuildInfoStore();
    await store.load();
    await wrapper.vm.$nextTick();
    // The version is not rendered when loaded=false. The copyright
    // year may still be present, so we just check the version is absent.
    expect(wrapper.text()).not.toContain("v0.0.0-dev");
  });
});
