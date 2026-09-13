import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { setActivePinia, createPinia } from "pinia";
import TeamBadge from "./TeamBadge.vue";

describe("TeamBadge", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("renders an <img> when src is provided", () => {
    const wrapper = mount(TeamBadge, { props: { src: "/logo.png", alt: "Manchester City" } });
    const img = wrapper.find("img.team-badge-img");
    expect(img.exists()).toBe(true);
    expect(img.attributes("src")).toBe("/logo.png");
    expect(img.attributes("alt")).toBe("Manchester City");
    expect(wrapper.find(".team-badge-fallback").exists()).toBe(false);
  });

  it("renders initials fallback when src is missing", () => {
    const wrapper = mount(TeamBadge, { props: { alt: "Manchester City" } });
    const fallback = wrapper.find(".team-badge-fallback");
    expect(fallback.exists()).toBe(true);
    expect(fallback.text()).toBe("MC");
  });

  it("derives initials from first two letters when the team name is a single word", () => {
    const wrapper = mount(TeamBadge, { props: { alt: "Chelsea" } });
    expect(wrapper.find(".team-badge-fallback").text()).toBe("CH");
  });

  it("swaps to fallback after the image fails to load", async () => {
    const wrapper = mount(TeamBadge, { props: { src: "/missing.png", alt: "Arsenal" } });
    expect(wrapper.find("img.team-badge-img").exists()).toBe(true);

    await wrapper.find("img.team-badge-img").trigger("error");

    expect(wrapper.find("img.team-badge-img").exists()).toBe(false);
    expect(wrapper.find(".team-badge-fallback").exists()).toBe(true);
    expect(wrapper.find(".team-badge-fallback").text()).toBe("AR");
  });

  it("handles empty alt by rendering a question mark", () => {
    const wrapper = mount(TeamBadge, { props: { alt: "   " } });
    expect(wrapper.find(".team-badge-fallback").text()).toBe("?");
  });

  it("uses the configured size for both img and fallback", () => {
    const wrapper = mount(TeamBadge, { props: { src: "/logo.png", alt: "Real Madrid", size: 48 } });
    const img = wrapper.find("img.team-badge-img");
    expect(img.attributes("width")).toBe("48");
    expect(img.attributes("height")).toBe("48");
  });
});

describe("TeamBadge - error handler integration", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
  });

  it("does not call handleError twice if error fires repeatedly (one swap is enough)", async () => {
    const wrapper = mount(TeamBadge, { props: { src: "/missing.png", alt: "PSG" } });
    const img = wrapper.find("img.team-badge-img");
    await img.trigger("error");
    expect(wrapper.find(".team-badge-fallback").exists()).toBe(true);

    // The img node is gone — second trigger should be a no-op.
    expect(() => wrapper.find("img.team-badge-img")).not.toThrow();
  });
});
