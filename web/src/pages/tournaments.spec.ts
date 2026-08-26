import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { h } from "vue";
import { setActivePinia, createPinia } from "pinia";
import tournaments from "./tournaments.vue";
import TournamentFormModal from "./TournamentFormModal.vue";

vi.mock("../store/services", () => ({
  tournamentsApiService: {
    getTournamentPage: vi.fn().mockResolvedValue({
      data: [{ id: 1, name: "Test", slug: "tst" }],
      page: { nextCursor: "", hasMore: false },
    }),
    createTournament: vi.fn().mockResolvedValue({}),
    updateTournament: vi.fn().mockResolvedValue({}),
    deleteTournament: vi.fn().mockResolvedValue({}),
  },
}));

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
  });
  await router.push("/");
  await router.isReady();
  return mount(tournaments, { global: { plugins: [router] } });
}

describe("tournaments.vue", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("renders a Crear button in the header", async () => {
    const wrapper = await mountPage();
    expect(wrapper.text()).toContain("Crear");
  });

  it("clicking Crear opens the modal in create mode", async () => {
    const wrapper = await mountPage();
    await flushPromises();
    const buttons = wrapper.findAll("button");
    const crearBtn = buttons.find((b) => b.text() === "Crear");
    expect(crearBtn).toBeTruthy();
    await crearBtn!.trigger("click");
    await flushPromises();
    const modal = wrapper.findComponent(TournamentFormModal);
    const modalState = (modal.vm as unknown as { modal: { open: boolean } }).modal;
    expect(modalState.open).toBe(true);
  });

  it("clicking Editar on a row opens the modal pre-filled", async () => {
    const wrapper = await mountPage();
    await flushPromises();
    const editBtn = wrapper.findAll("button").find((b) => b.text() === "Editar");
    expect(editBtn).toBeTruthy();
    await editBtn!.trigger("click");
    await flushPromises();
    const modal = wrapper.findComponent(TournamentFormModal);
    const form = (modal.vm as unknown as { modal: { form: { id: number; name: string; slug: string } } }).modal.form;
    expect(form.id).toBe(1);
    expect(form.name).toBe("Test");
    expect(form.slug).toBe("tst");
  });
});
