import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
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
