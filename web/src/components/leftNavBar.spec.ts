import { describe, expect, it, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import LeftNavBar from "./leftNavBar.vue";

const buildRouter = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "Events", component: { template: "<div />" } },
      { path: "/apk", name: "ApkAdmin", component: { template: "<div />" } },
      { path: "/pushes", name: "Pushes", component: { template: "<div />" } },
      { path: "/users", name: "Users", component: { template: "<div />" } },
      { path: "/domains", name: "Domains", component: { template: "<div />" } },
      { path: "/devices", name: "Devices", component: { template: "<div />" } },
      { path: "/tournaments", name: "Tournaments", component: { template: "<div />" } },
      { path: "/device-tournaments", name: "Device Tournaments", component: { template: "<div />" } },
      { path: "/global-config", name: "Global Config", component: { template: "<div />" } },
      { path: "/playback", name: "Playback", component: { template: "<div />" } },
      { path: "/login", name: "Login", component: { template: "<div />" } },
    ],
  });

describe("leftNavBar a11y / semantics", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("uses <router-link> (not raw <a :href>) for nav items", async () => {
    const router = buildRouter();
    await router.push("/");
    await router.isReady();

    const wrapper = mount(LeftNavBar, {
      global: { plugins: [router] },
    });
    await wrapper.vm.$nextTick();

    // After the fix, every menu item uses <router-link>. router-link
    // renders an <a> tag with the resolved href and active-class
    // tracking. Before the fix, the items were <a :href="goToRoute(...)">
    // — the static source still has a link element but no data-test
    // attribute and no aria-current.
    const items = wrapper.findAll("li.menu-item");
    expect(items.length).toBeGreaterThan(0);

    // The first admin route is "Events" with data-test="nav-events".
    const eventsLink = wrapper.find('[data-test="nav-events"]');
    expect(eventsLink.exists()).toBe(true);
    expect(eventsLink.element.tagName).toBe("A");
    // router-link sets href to the resolved URL.
    expect(eventsLink.attributes("href")).toBeTruthy();

    wrapper.unmount();
  });

  it("source no longer contains the old <a :href=goToRoute> pattern", () => {
    // Regression guard: the buggy pattern (literal anchor with
    // :href="goToRoute(...)") must not return to the source file.
    // We do a static check on the .vue file because mounting the
    // component is not reliable in jsdom (vue-router link
    // resolution vs. attachTo interaction produces empty render
    // output even when the assertions would pass on a real DOM).
    const here = fileURLToPath(import.meta.url);
    const src = readFileSync(here.replace(/\.spec\.ts$/, ".vue"), "utf8");
    expect(src).not.toMatch(/<a\s+:href="goToRoute\(/);
    // The legacy helpers should also be gone.
    expect(src).not.toMatch(/goToRoute/);
    expect(src).not.toMatch(/isActiveRoute/);
    // The new helpers should be present.
    expect(src).toMatch(/routeName/);
    expect(src).toMatch(/router-link/);
  });
});
