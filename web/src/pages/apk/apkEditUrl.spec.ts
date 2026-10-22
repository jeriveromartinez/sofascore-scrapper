import { describe, it, expect, beforeEach, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import apkEditUrl from "./apkEditUrl.vue";
import apkUploadModal from "./apkUploadModal.vue";
import { apkApiService } from "../../store/services";
import type { ApkInfo } from "../../proto/api";
import { setActivePinia, createPinia } from "pinia";

vi.mock("../../store/services", () => ({
  apkApiService: {
    updateApkUrl: vi.fn().mockResolvedValue({ message: "ok" }),
    resumeSession: vi.fn().mockResolvedValue(null),
    uploadApk: vi.fn().mockResolvedValue({ version: "1.0.0" }),
  },
}));

const mockInfo: ApkInfo = {
  id: 42,
  version: "1.0.0",
  fileName: "",
  fileSize: 100,
  description: "",
  packageName: "com.example",
  versionCode: 0,
  minSdkVersion: 0,
  targetSdkVersion: 0,
  downloadToken: "tok",
  downloadUrl: "",
  createdAt: "0",
  isActive: true,
  downloads: 0,
  panelUrl: "https://original.example.com",
};

type ModalState = { info: ApkInfo };


describe("apkEditUrl.vue", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("clones data in openModal so parent row is not mutated", () => {
    const wrapper = mount(apkEditUrl);
    const vm = wrapper.vm as any;
    const modal = vm.modal as ModalState;

    const info = { ...mockInfo };
    vm.openModal(info);

    expect(info.panelUrl).toBe("https://original.example.com");
    expect(modal.info.panelUrl).toBe("https://original.example.com");

    modal.info.panelUrl = "https://modified.example.com";

    expect(info.panelUrl).toBe("https://original.example.com");
    expect(modal.info.panelUrl).toBe("https://modified.example.com");
  });

  it("does not mutate parent on cancel (closeModal resets state)", () => {
    const wrapper = mount(apkEditUrl);
    const vm = wrapper.vm as any;
    const modal = vm.modal as ModalState;

    const info = { ...mockInfo };
    vm.openModal(info);
    modal.info.panelUrl = "https://changed-before-cancel.example.com";
    vm.closeModal();

    expect(info.panelUrl).toBe("https://original.example.com");
    expect(modal.info.id).toBeUndefined();
  });

  it("submits its own form when the upload modal is open on the same page", async () => {
    const Host = defineComponent({
      setup() {
        return () => [h(apkUploadModal), h(apkEditUrl, { ref: "edit" })];
      },
    });
    const wrapper = mount(Host, { attachTo: document.body });
    await wrapper.find("button.btn-outline-primary").trigger("click");
    await flushPromises();
    (wrapper.vm.$refs.edit as { openModal: (info: ApkInfo) => void }).openModal({ ...mockInfo });
    await flushPromises();

    const updateButton = wrapper
      .findAll("button[type='submit']")
      .find((b) => b.text() === "Update");
    (updateButton!.element as HTMLButtonElement).click();
    await flushPromises();

    expect(apkApiService.updateApkUrl).toHaveBeenCalledWith(42, "https://original.example.com");
    expect(apkApiService.uploadApk).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
