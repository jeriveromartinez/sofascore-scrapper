import { nextTick } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
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

describe("DomainFormModal", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("renders the user <select> from the users prop", async () => {
    const wrapper = mount(DomainFormModal, { props: { users } });
    const vm = wrapper.vm as unknown as { open: (d?: Domain) => void };
    vm.open();
    await nextTick();
    const options = wrapper.findAll("select option");
    expect(options.length).toBe(3);
  });

  it("open() without args puts modal in create mode and selects first user as default", () => {
    const wrapper = mount(DomainFormModal, { props: { users } });
    const vm = wrapper.vm as unknown as {
      open: (d?: Domain) => void;
      modal: { form: { domain: string; userId: number; id: number | null } };
    };
    vm.open();
    expect(vm.modal.form.id).toBeNull();
    expect(vm.modal.form.userId).toBe(10);
  });

  it("open(entity) pre-fills domain + userId and clones the entity", () => {
    const wrapper = mount(DomainFormModal, { props: { users } });
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

  it("submit in create mode emits submit event with id=null", async () => {
    const wrapper = mount(DomainFormModal, { props: { users } });
    const vm = wrapper.vm as unknown as {
      open: (d?: Domain) => void;
      modal: { form: { domain: string; userId: number; id: number | null } };
      submit: () => Promise<void>;
    };
    vm.open();
    vm.modal.form.domain = "new.com";
    vm.modal.form.userId = 20;

    await vm.submit();
    const emitted = wrapper.emitted("submit");
    expect(emitted).toBeTruthy();
    expect(emitted![0]![0]).toEqual({ id: null, domain: "new.com", userId: 20 });
  });
});