import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { toast } from "vue3-toastify";
import { authApiService } from "../store/services";
import { createPinia, setActivePinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import Register from "./register.vue";

vi.mock("../store/services", () => ({
  authApiService: {
    login: vi.fn(),
    register: vi.fn().mockResolvedValue({ id: 1, email: "u@x.com", token: "t", refreshToken: "rt" }),
    logout: vi.fn(),
  },
}));

vi.mock("vue3-toastify", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const buildRouter = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "Events", component: { template: "<div />" } },
      { path: "/login", name: "Login", component: { template: "<div />" } },
      { path: "/register", name: "Register", component: Register },
    ],
  });

describe("register.vue a11y", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("uses type=email with autocomplete=email on the email input", () => {
    const wrapper = mount(Register, {
      global: { plugins: [buildRouter()] },
      attachTo: document.body,
    });
    const input = wrapper.find<HTMLInputElement>('input[name="email"]');
    expect(input.exists()).toBe(true);
    expect(input.attributes("type")).toBe("email");
    expect(input.attributes("autocomplete")).toBe("email");
    wrapper.unmount();
  });

  it("renders the password reveal toggle as a <button> with aria-pressed", () => {
    const wrapper = mount(Register, {
      global: { plugins: [buildRouter()] },
      attachTo: document.body,
    });
    const toggle = wrapper.find('[data-test="password-reveal"]');
    expect(toggle.exists()).toBe(true);
    expect(toggle.element.tagName).toBe("BUTTON");
    expect(toggle.attributes("type")).toBe("button");
    expect(toggle.attributes("aria-pressed")).toBe("false");
    expect(toggle.attributes("aria-label")).toBeTruthy();
    wrapper.unmount();
  });

  it("does not reference its own id via aria-describedby", () => {
    // Per WAI-ARIA: aria-describedby must point to an external helper
    // text element. Pointing it at the input's own id ("password")
    // means screen readers will look for an id="password" element to
    // describe the field and find nothing useful.
    const wrapper = mount(Register, {
      global: { plugins: [buildRouter()] },
      attachTo: document.body,
    });
    const password = wrapper.find<HTMLInputElement>('input[name="password"]');
    const describedBy = password.attributes("aria-describedby");
    if (describedBy) {
      expect(describedBy).not.toBe("password");
      // If aria-describedby is set, the referenced element must exist
      // in the document.
      expect(document.getElementById(describedBy)).not.toBeNull();
    }
    wrapper.unmount();
  });
});

describe("register.vue submission guard", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  async function fillAndSubmit(fields: { email: string; password: string; agree: boolean }) {
    const wrapper = mount(Register, { global: { plugins: [buildRouter()] } });
    await wrapper.find('input[name="email"]').setValue(fields.email);
    await wrapper.find('input[name="password"]').setValue(fields.password);
    await wrapper.find('input[name="terms"]').setValue(fields.agree);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    return wrapper;
  }

  it("marks email, password and consent as required", () => {
    const wrapper = mount(Register, { global: { plugins: [buildRouter()] } });
    for (const name of ["email", "password", "terms"]) {
      expect(wrapper.find(`input[name="${name}"]`).attributes("required")).toBeDefined();
    }
  });

  it("does not register when the terms are not accepted", async () => {
    await fillAndSubmit({ email: "u@x.com", password: "a-long-password", agree: false });
    expect(authApiService.register).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalled();
  });

  it("does not register with an empty email or password", async () => {
    await fillAndSubmit({ email: "", password: "", agree: true });
    expect(authApiService.register).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalled();
  });

  it("registers when the fields are filled and the terms are accepted", async () => {
    await fillAndSubmit({ email: "u@x.com", password: "a-long-password", agree: true });
    expect(authApiService.register).toHaveBeenCalledWith("u@x.com", "a-long-password", "");
  });
});
