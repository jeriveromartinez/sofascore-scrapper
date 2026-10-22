import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
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

const onSubmit = vi.fn();

beforeEach(() => {
  onSubmit.mockReset();
});

describe("TournamentFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("open() without args puts modal in create mode (no id emitted)", async () => {
    const wrapper = mount(TournamentFormModal, { props: { onSubmit } });
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
    const wrapper = mount(TournamentFormModal, { props: { onSubmit } });
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

  it("submit in create mode calls onSubmit without id", async () => {
    const wrapper = mount(TournamentFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { form: { name: string; slug: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.name = "Copa";
    vm.modal.form.slug = "copa";

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledWith({ id: null, name: "Copa", slug: "copa" });
  });

  it("submit in edit mode calls onSubmit with id", async () => {
    const wrapper = mount(TournamentFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (t?: Tournament) => void;
      modal: { form: { name: string; slug: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open({ ...base });
    vm.modal.form.name = "Premier League";

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledWith({ id: 1, name: "Premier League", slug: "premier" });
  });
});

describe("TournamentFormModal form submission", () => {
  it("associates the submit button with the form and submits exactly once", async () => {
    const wrapper = mount(TournamentFormModal, { props: { onSubmit }, attachTo: document.body });
    (wrapper.vm as unknown as { open: () => void }).open();
    await wrapper.vm.$nextTick();
    const button = wrapper.find("button[type='submit']").element as HTMLButtonElement;
    expect(button.form).toBe(wrapper.find("form").element);
    await wrapper.find("input[required]").setValue("Serie A");
    button.click();
    await flushPromises();
    expect(onSubmit).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});

describe("TournamentFormModal async submit", () => {
  type Vm = {
    open: () => void;
    submit: () => Promise<void>;
    modal: { loading: boolean; error: string; form: Record<string, unknown> };
  };

  it("keeps the submit button disabled until the parent handler settles", async () => {
    let finish!: () => void;
    const onSubmit = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    const wrapper = mount(TournamentFormModal, { props: { onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { name: "Copa" });
    const pending = vm.submit();
    await flushPromises();

    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(vm.modal.loading).toBe(true);
    expect(wrapper.find("button[type='submit']").attributes("disabled")).toBeDefined();

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledTimes(1);

    finish();
    await pending;
    expect(vm.modal.loading).toBe(false);
  });

  it("shows the error when the parent handler rejects", async () => {
    const onSubmit = vi.fn().mockRejectedValue(new Error("boom"));
    const wrapper = mount(TournamentFormModal, { props: { onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { name: "Copa" });
    await vm.submit();
    expect(vm.modal.error).toBe("boom");
    expect(vm.modal.loading).toBe(false);
  });
});
