import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import UserFormModal from "./UserFormModal.vue";
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

describe("UserFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("open() without args puts modal in create mode and requires password", async () => {
    const wrapper = mount(UserFormModal);
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
    const wrapper = mount(UserFormModal);
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
    const emitted = wrapper.emitted("submit");
    expect(emitted).toBeTruthy();
    expect(emitted![0]![0]).toEqual({ id: 1, email: "admin@x.com", password: "", role: "admin" });
  });

  it("exposes a role select with user and admin options in edit mode", async () => {
    const wrapper = mount(UserFormModal);
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
    const wrapper = mount(UserFormModal);
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
    };
    vm.open();
    await wrapper.vm.$nextTick();
    expect(wrapper.find('select[data-testid="user-role"]').exists()).toBe(false);
  });

  it("submit in create mode with email + password emits payload", async () => {
    const wrapper = mount(UserFormModal);
    const vm = wrapper.vm as unknown as {
      open: (u?: User) => void;
      modal: { form: { email: string; password: string; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.email = "new@x.com";
    vm.modal.form.password = "secret";

    await vm.submit();
    const emitted = wrapper.emitted("submit");
    expect(emitted).toBeTruthy();
    expect(emitted![0]![0]).toEqual({ id: null, email: "new@x.com", password: "secret", role: "user" });
  });
});
