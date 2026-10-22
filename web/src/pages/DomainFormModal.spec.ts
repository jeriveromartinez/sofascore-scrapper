import { nextTick } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import DomainFormModal from "./DomainFormModal.vue";
import type { Domain, User } from "../store/services/models";

vi.mock("../store/services", () => ({
  domainsApiService: {
    createDomain: vi.fn().mockResolvedValue({ id: 1 }),
    updateDomain: vi.fn().mockResolvedValue({ id: 1 }),
  },
}));

const users: User[] = [
  { id: 10, email: "a@x.com", createdAt: "", updatedAt: "", role: "user", notificationsEnabled: false, notificationsEnabledAt: "" },
  { id: 20, email: "b@x.com", createdAt: "", updatedAt: "", role: "user", notificationsEnabled: false, notificationsEnabledAt: "" },
];

const base: Domain = {
  id: 1,
  domain: "example.com",
  userId: 10,
  user: users[0],
  createdAt: "",
  updatedAt: "",
};

const onSubmit = vi.fn();

beforeEach(() => {
  onSubmit.mockReset();
});

describe("DomainFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("renders the user <select> from the users prop", async () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit } });
    const vm = wrapper.vm as unknown as { open: (d?: Domain) => void };
    vm.open();
    await nextTick();
    const options = wrapper.findAll("select option");
    expect(options.length).toBe(3);
  });

  it("open() without args puts modal in create mode and selects first user as default", () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (d?: Domain) => void;
      modal: { form: { domain: string; userId: number; id: number | null } };
    };
    vm.open();
    expect(vm.modal.form.id).toBeNull();
    expect(vm.modal.form.userId).toBe(10);
  });

  it("open(entity) pre-fills domain + userId and clones the entity", () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (d?: Domain) => void;
      modal: { form: { domain: string; userId: number; id: number | null } };
    };
    vm.open({ ...base });
    expect(vm.modal.form.id).toBe(1);
    expect(vm.modal.form.domain).toBe("example.com");
    expect(vm.modal.form.userId).toBe(10);

    vm.modal.form.domain = "other.com";
    expect(base.domain).toBe("example.com");
  });

  it("submit in create mode calls onSubmit with id=null", async () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit } });
    const vm = wrapper.vm as unknown as {
      open: (d?: Domain) => void;
      modal: { form: { domain: string; userId: number; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.domain = "new.com";
    vm.modal.form.userId = 20;

    await vm.submit();
    expect(onSubmit).toHaveBeenCalledWith({ id: null, domain: "new.com", userId: 20 });
  });
});
describe("DomainFormModal form submission", () => {
  it("associates the submit button with the form and submits exactly once", async () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit }, attachTo: document.body });
    (wrapper.vm as unknown as { open: () => void }).open();
    await nextTick();
    const button = wrapper.find("button[type='submit']").element as HTMLButtonElement;
    expect(button.form).toBe(wrapper.find("form").element);
    await wrapper.find("input[type='text']").setValue("new.example.com");
    button.click();
    await flushPromises();
    expect(onSubmit).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("does not submit when the required domain is empty", async () => {
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit }, attachTo: document.body });
    (wrapper.vm as unknown as { open: () => void }).open();
    await nextTick();
    (wrapper.find("button[type='submit']").element as HTMLButtonElement).click();
    await flushPromises();
    expect(onSubmit).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe("DomainFormModal async submit", () => {
  type Vm = {
    open: () => void;
    submit: () => Promise<void>;
    modal: { loading: boolean; error: string; form: Record<string, unknown> };
  };

  it("keeps the submit button disabled until the parent handler settles", async () => {
    let finish!: () => void;
    const onSubmit = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { domain: "new.com", userId: 20 });
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
    const wrapper = mount(DomainFormModal, { props: { users, onSubmit, autoCloseModal: false } });
    const vm = wrapper.vm as unknown as Vm;
    vm.open();
    Object.assign(vm.modal.form, { domain: "new.com", userId: 20 });
    await vm.submit();
    expect(vm.modal.error).toBe("boom");
    expect(vm.modal.loading).toBe(false);
  });
});
