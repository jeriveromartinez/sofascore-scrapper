import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import TournamentFormModal from "./TournamentFormModal.vue";
import type { Tournament } from "../store/services/models";

vi.mock("../store/services", () => ({
  tournamentsApiService: {
    createTournament: vi.fn().mockResolvedValue({ id: 99, name: "X", slug: "x" }),
    updateTournament: vi.fn().mockResolvedValue({ id: 99, name: "X", slug: "x" }),
  },
}));

const base: Tournament = {
  id: 1,
  createdAt: "2024-01-01T00:00:00Z",
  updatedAt: "2024-01-01T00:00:00Z",
  name: "Premier",
  slug: "premier",
  region: "EU",
};

describe("TournamentFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("open() without args puts modal in create mode (no id emitted)", async () => {
    const wrapper = mount(TournamentFormModal);
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { open: boolean; form: { name: string; slug: string; id: number | null } };
      submit: () => Promise<void>;
    };

    vm.open();
    expect(vm.modal.open).toBe(true);
    expect(vm.modal.form.id).toBeNull();
  });

  it("open(entity) pre-fills the form and clones the entity (no parent mutation)", async () => {
    const wrapper = mount(TournamentFormModal);
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { form: { name: string; slug: string; id: number | null } };
    };

    vm.open({ ...base });
    expect(vm.modal.form.name).toBe("Premier");
    expect(vm.modal.form.slug).toBe("premier");
    expect(vm.modal.form.id).toBe(1);

    vm.modal.form.name = "Changed";
    expect(base.name).toBe("Premier");
  });

  it("submit in create mode emits submit event without id", async () => {
    const wrapper = mount(TournamentFormModal);
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { form: { name: string; slug: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.name = "Copa";
    vm.modal.form.slug = "copa";

    await vm.submit();
    const emitted = wrapper.emitted("submit");
    expect(emitted).toBeTruthy();
    expect(emitted![0]![0]).toEqual({ id: null, name: "Copa", slug: "copa" });
  });

  it("submit in edit mode emits submit event with id", async () => {
    const wrapper = mount(TournamentFormModal);
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { form: { name: string; slug: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open({ ...base });
    vm.modal.form.name = "Premier League";

    await vm.submit();
    const emitted = wrapper.emitted("submit");
    expect(emitted).toBeTruthy();
    expect(emitted![0]![0]).toEqual({ id: 1, name: "Premier League", slug: "premier" });
  });
});
