import { describe, it, expect, beforeEach, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import apkUploadModal from "./apkUploadModal.vue";
import { apkApiService } from "../../store/services";

vi.mock("../../store/services", () => ({
  apkApiService: {
    resumeSession: vi.fn(),
  },
}));

describe("apkUploadModal.vue", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows an error when checking for a pending upload fails", async () => {
    vi.mocked(apkApiService.resumeSession).mockRejectedValue(new Error("status unavailable"));
    const wrapper = mount(apkUploadModal);

    await wrapper.find("button.btn-outline-primary").trigger("click");
    await flushPromises();

    expect(wrapper.find(".alert-danger").text()).toContain("status unavailable");
    wrapper.unmount();
  });
});
