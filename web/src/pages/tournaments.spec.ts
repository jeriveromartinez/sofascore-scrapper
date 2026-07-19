import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import tournaments from "./tournaments.vue";
import { setActivePinia, createPinia } from "pinia";

vi.mock("../store/services", () => ({
  tournamentsApiService: {
    getAllTournaments: vi.fn().mockResolvedValue([]),
    createTournament: vi.fn().mockResolvedValue({}),
    updateTournament: vi.fn().mockResolvedValue({}),
    deleteTournament: vi.fn().mockResolvedValue({}),
  },
}));

describe("tournaments.vue", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("renders the form regardless of editing state", () => {
    const wrapper = mount(tournaments);
    expect(wrapper.find("form.row").exists()).toBe(true);
  });

  it("shows Create button when not editing", () => {
    const wrapper = mount(tournaments);
    expect(wrapper.text()).toContain("Crear");
  });

  it("shows Update and Cancel when editing", async () => {
    const wrapper = mount(tournaments);
    await flushPromises();

    (wrapper.vm as any).startEdit({ id: 1, name: "Test", slug: "tst" });
    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain("Actualizar");
    expect(wrapper.text()).toContain("Cancelar");
  });

  it("cancelEdit clears form and editingId", async () => {
    const wrapper = mount(tournaments);
    await flushPromises();

    (wrapper.vm as any).startEdit({ id: 1, name: "Test", slug: "tst" });
    await wrapper.vm.$nextTick();

    (wrapper.vm as any).cancelEdit();
    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain("Crear");
  });
});
