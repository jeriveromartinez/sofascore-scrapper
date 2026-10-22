import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import UserFormModal from "./UserFormModal.vue";
import TournamentFormModal from "./TournamentFormModal.vue";
import type { User } from "../store/services/models";

vi.mock("../store/services", () => ({
  usersApiService: {
    createUser: vi.fn().mockResolvedValue({ id: 1, email: "a@b.c" }),
    updateUser: vi.fn().mockResolvedValue({ id: 1, email: "a@b.c" }),
  },
}));

const base: User = {
  id: 1,
  createdAt: "2024-01-01T00:00:00Z",
  updatedAt: "2024-01-01T00:00:00Z",
  email: "admin@x.com",
  role: "admin",
  notificationsEnabled: false,
  notificationsEnabledAt: "",
};

enableAutoUnmount(afterEach);

const onSubmit = vi.fn();

beforeEach(() => {
  onSubmit.mockReset();
});

describe("UserFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("open() without args puts modal in create mode and requires password", async () => {
    const wrapper = mount(UserFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
      modal: { open: boolean; form: { email: string; password: string; id: number | null }; error: string };
      submit: () => Promise<void>;
    };
    vm.open();
    expect(vm.modal.open).toBe(true);
    expect(vm.modal.form.id).toBeNull();

    vm.modal.form.email = "new@x.com";
    await vm.submit();
    expect(vm.modal.error).toMatch(/contraseña/);
  });

  it("open(user) pre-fills email, role, leaves password blank, makes password optional", async () => {
    const wrapper = mount(UserFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
      modal: { form: { email: string; password: string; id: number | null; role: string }; error: string };
      submit: () => Promise<void>;
    };
    vm.open({ ...base });
    expect(vm.modal.form.email).toBe("admin@x.com");
    expect(vm.modal.form.password).toBe("");
    expect(vm.modal.form.id).toBe(1);
    expect(vm.modal.form.role).toBe("admin");

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledWith({ id: 1, email: "admin@x.com", password: "", role: "admin" });
  });

  it("exposes a role select with user and admin options in edit mode", async () => {
    const wrapper = mount(UserFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
      modal: { form: { id: number | null } };
    };
    vm.open({ ...base });
    await wrapper.vm.$nextTick();
    const select = wrapper.find('select[data-testid="user-role"]');
    expect(select.exists()).toBe(true);
    const options = select.findAll("option").map((o) => (o.element as HTMLOptionElement).value);
    expect(options).toEqual(["user", "admin"]);
    expect((select.element as HTMLSelectElement).value).toBe("admin");
  });

  it("does not render the role select in create mode", async () => {
    const wrapper = mount(UserFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
    };
    vm.open();
    await wrapper.vm.$nextTick();
    expect(wrapper.find('select[data-testid="user-role"]').exists()).toBe(false);
  });

  it("submit in create mode with email + password calls onSubmit with the payload", async () => {
    const wrapper = mount(UserFormModal, { props: { onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
      modal: { form: { email: string; password: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.email = "new@x.com";
    vm.modal.form.password = "secret";

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledWith({ id: null, email: "new@x.com", password: "secret", role: "user" });
  });
});

describe("UserFormModal form submission", () => {
  async function openCreate() {
    const wrapper = mount(UserFormModal, { props: { onSubmit }, attachTo: document.body });
    (wrapper.vm as unknown as { open: () => void }).open();
    await wrapper.vm.$nextTick();
    return wrapper;
  }

  function clickSubmit(wrapper: ReturnType<typeof mount>) {
    const button = wrapper.find("button[type='submit']").element as HTMLButtonElement;
    button.click();
  }

  it("runs native validation before submitting (invalid email is rejected)", async () => {
    const wrapper = await openCreate();
    await wrapper.find("input[type='email']").setValue("not-an-email");
    await wrapper.find("input[type='password']").setValue("secret");
    clickSubmit(wrapper);
    await flushPromises();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("associates the submit button with the form and submits exactly once", async () => {
    const wrapper = await openCreate();
    const button = wrapper.find("button[type='submit']").element as HTMLButtonElement;
    expect(button.form).toBe(wrapper.find("form").element);
    await wrapper.find("input[type='email']").setValue("new@x.com");
    await wrapper.find("input[type='password']").setValue("secret");
    clickSubmit(wrapper);
    await flushPromises();
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});

describe("UserFormModal async submit", () => {
  type Vm = {
    open: () => void;
    submit: () => Promise<void>;
    modal: { loading: boolean; error: string; form: Record<string, unknown> };
  };

  it("keeps the submit button disabled until the parent handler settles", async () => {
    let finish!: () => void;
    const onSubmit = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    const wrapper = mount(UserFormModal, { props: { onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { email: "new@x.com", password: "secret" });
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
    const wrapper = mount(UserFormModal, { props: { onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { email: "new@x.com", password: "secret" });
    await vm.submit();
    expect(vm.modal.error).toBe("boom");
    expect(vm.modal.loading).toBe(false);
  });
});

describe("UserFormModal modal behaviour", () => {
  it("closes on Escape and restores focus to the opener", async () => {
    const opener = document.createElement("button");
    document.body.appendChild(opener);
    opener.focus();
    const wrapper = mount(UserFormModal, { props: { onSubmit }, attachTo: document.body });
    const vm = wrapper.vm as unknown as { open: () => void; modal: { open: boolean } };
    vm.open();
    await flushPromises();
    expect(wrapper.find("[role='dialog']").element.contains(document.activeElement)).toBe(true);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();

    expect(vm.modal.open).toBe(false);
    expect(document.activeElement).toBe(opener);
    opener.remove();
  });

  it("keeps page scroll locked while another modal is still open", async () => {
    const users = mount(UserFormModal, { props: { onSubmit }, attachTo: document.body });
    const tournaments = mount(TournamentFormModal, { props: { onSubmit }, attachTo: document.body });
    const usersVm = users.vm as unknown as { open: () => void; reset: () => void };
    const tournamentsVm = tournaments.vm as unknown as { open: () => void; reset: () => void };
    usersVm.open();
    tournamentsVm.open();
    await flushPromises();

    tournamentsVm.reset();
    await flushPromises();
    expect(document.body.style.overflow).toBe("hidden");

    usersVm.reset();
    await flushPromises();
    expect(document.body.style.overflow).toBe("");
  });
});
