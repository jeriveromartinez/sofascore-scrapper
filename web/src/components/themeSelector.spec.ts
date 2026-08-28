import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import ThemeSelector from "./themeSelector.vue";
import { useStyleStore } from "../store/pinia/styleStore";

describe("themeSelector", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
    document.documentElement.removeAttribute("data-bs-theme");
  });

  it("renders a <button> as the dropdown trigger, not an <a>", () => {
    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const trigger = wrapper.find('[data-test="theme-trigger"]');
    expect(trigger.exists()).toBe(true);
    expect(trigger.element.tagName).toBe("BUTTON");
    expect(trigger.element.getAttribute("type")).toBe("button");
    wrapper.unmount();
  });

  it("binds aria-expanded to the dropdown state and reflects false initially", () => {
    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const trigger = wrapper.find('[data-test="theme-trigger"]');
    expect(trigger.attributes("aria-expanded")).toBe("false");
    expect(trigger.attributes("aria-haspopup")).toBe("menu");
    wrapper.unmount();
  });

  it("flips aria-expanded to true after the trigger is clicked", async () => {
    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const trigger = wrapper.find('[data-test="theme-trigger"]');
    await trigger.trigger("click");
    expect(trigger.attributes("aria-expanded")).toBe("true");
    wrapper.unmount();
  });

  it("marks the icon as aria-hidden so screen readers skip the glyph", () => {
    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const icon = wrapper.find('[data-test="theme-icon"]');
    expect(icon.exists()).toBe(true);
    expect(icon.attributes("aria-hidden")).toBe("true");
    wrapper.unmount();
  });

  it("reflects the current theme in the trigger's aria-label", () => {
    const styleStore = useStyleStore();
    styleStore.setTheme("light");

    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const trigger = wrapper.find('[data-test="theme-trigger"]');
    const label = trigger.attributes("aria-label") ?? "";
    expect(label.toLowerCase()).toContain("light");
    wrapper.unmount();
  });

  it("marks the active theme option with aria-pressed=true and the inactive one with aria-pressed=false", () => {
    const styleStore = useStyleStore();
    styleStore.setTheme("dark");

    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const lightBtn = wrapper.find('[data-bs-theme-value="light"]');
    const darkBtn = wrapper.find('[data-bs-theme-value="dark"]');

    expect(lightBtn.attributes("aria-pressed")).toBe("false");
    expect(darkBtn.attributes("aria-pressed")).toBe("true");
    wrapper.unmount();
  });

  it("switches aria-pressed to the new active theme after a click", async () => {
    const styleStore = useStyleStore();
    styleStore.setTheme("dark");

    const wrapper = mount(ThemeSelector, { attachTo: document.body });
    const lightBtn = wrapper.find('[data-bs-theme-value="light"]');
    await lightBtn.trigger("click");
    // setTheme runs synchronously, so the next tick re-renders the buttons.
    await wrapper.vm.$nextTick();

    const updatedLight = wrapper.find('[data-bs-theme-value="light"]');
    const updatedDark = wrapper.find('[data-bs-theme-value="dark"]');
    expect(updatedLight.attributes("aria-pressed")).toBe("true");
    expect(updatedDark.attributes("aria-pressed")).toBe("false");
    wrapper.unmount();
  });
});
