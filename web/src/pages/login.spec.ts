import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import Login from "./login.vue";

vi.mock("../store/services", () => ({
  authApiService: {
    login: vi.fn().mockResolvedValue({ id: 1, email: "u@x.com", token: "t", refreshToken: "rt" }),
    register: vi.fn(),
    logout: vi.fn(),
  },
}));

const flush = () => new Promise((r) => setTimeout(r, 0));

const buildRouter = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "Events", component: { template: "<div />" } },
      { path: "/register", name: "Register", component: { template: "<div />" } },
      { path: "/login", name: "Login", component: Login },
    ],
  });

describe("login.vue a11y", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("uses type=email with autocomplete=email on the email input", () => {
    const wrapper = mount(Login, {
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
    const wrapper = mount(Login, {
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

  it("flips aria-pressed to true after a click and toggles input type", async () => {
    const wrapper = mount(Login, {
      global: { plugins: [buildRouter()] },
      attachTo: document.body,
    });
    const toggle = wrapper.find('[data-test="password-reveal"]');
    const password = wrapper.find<HTMLInputElement>('input[name="password"]');
    expect(password.attributes("type")).toBe("password");

    await toggle.trigger("click");
    await flush();

    expect(toggle.attributes("aria-pressed")).toBe("true");
    expect(password.attributes("type")).toBe("text");

    await toggle.trigger("click");
    await flush();
    expect(toggle.attributes("aria-pressed")).toBe("false");
    expect(password.attributes("type")).toBe("password");
    wrapper.unmount();
  });
});
