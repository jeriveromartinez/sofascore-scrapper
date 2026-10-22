import { describe, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import EventsFilterBar from "./EventsFilterBar.vue";
import type { EventsPageFilters } from "../store/services/models/apiModels";

function mountBar(filters: EventsPageFilters) {
  const onUpdate = vi.fn();
  const wrapper = mount(EventsFilterBar, {
    props: { modelValue: filters, "onUpdate:modelValue": onUpdate },
  });
  return { wrapper, onUpdate };
}

describe("EventsFilterBar", () => {
  it("renders ASC/DESC toggle bound to filters.direction", async () => {
    const { wrapper } = mountBar({ direction: "asc" });
    const buttons = wrapper.findAll("button");
    const ascBtn = buttons.find((b) => b.text().includes("ASC"));
    expect(ascBtn).toBeTruthy();
    expect(ascBtn!.classes()).toContain("active");
  });

  it("clicking DESC emits update with direction=desc", async () => {
    const { wrapper, onUpdate } = mountBar({ direction: "asc" });
    const descBtn = wrapper.findAll("button").find((b) => b.text().includes("DESC"));
    expect(descBtn).toBeTruthy();
    await descBtn!.trigger("click");
    await flushPromises();
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ direction: "desc" }));
  });

  it("clicking ASC keeps direction=asc instead of toggling it away", async () => {
    const { wrapper, onUpdate } = mountBar({ direction: "asc" });
    const ascBtn = wrapper.findAll("button").find((b) => b.text().includes("ASC"));
    expect(ascBtn).toBeTruthy();
    await ascBtn!.trigger("click");
    await flushPromises();
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ direction: "asc" }));
  });

  it("clicking DESC keeps direction=desc instead of toggling it away", async () => {
    const { wrapper, onUpdate } = mountBar({ direction: "desc" });
    const descBtn = wrapper.findAll("button").find((b) => b.text().includes("DESC"));
    expect(descBtn).toBeTruthy();
    await descBtn!.trigger("click");
    await flushPromises();
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ direction: "desc" }));
  });

  it("text inputs do not emit until 2+ chars", async () => {
    vi.useFakeTimers();
    try {
      const { wrapper, onUpdate } = mountBar({ direction: "asc" });
      const leagueInput = wrapper.find('input[name="league"]');
      await leagueInput.setValue("P");
      await flushPromises();
      expect(onUpdate).not.toHaveBeenCalled();

      await leagueInput.setValue("Pr");
      vi.advanceTimersByTime(400);
      await flushPromises();
      expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ league: "Pr" }));
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not emit from a pending debounce after the component is unmounted", async () => {
    vi.useFakeTimers();
    try {
      const { wrapper, onUpdate } = mountBar({ direction: "asc" });
      const leagueInput = wrapper.find('input[name="league"]');
      await leagueInput.setValue("Pr");
      await flushPromises();
      expect(onUpdate).not.toHaveBeenCalled();

      wrapper.unmount();
      vi.advanceTimersByTime(400);
      await flushPromises();

      expect(onUpdate).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
