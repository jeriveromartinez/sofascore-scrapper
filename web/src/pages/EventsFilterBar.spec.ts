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
  it("renders ASC/DESC toggle bound to filters.dir", async () => {
    const { wrapper } = mountBar({ dir: "asc" });
    const buttons = wrapper.findAll("button");
    const ascBtn = buttons.find((b) => b.text().includes("ASC"));
    expect(ascBtn).toBeTruthy();
    expect(ascBtn!.classes()).toContain("active");
  });

  it("clicking DESC emits update with dir=desc", async () => {
    const { wrapper, onUpdate } = mountBar({ dir: "asc" });
    const descBtn = wrapper.findAll("button").find((b) => b.text().includes("DESC"));
    expect(descBtn).toBeTruthy();
    await descBtn!.trigger("click");
    await flushPromises();
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ dir: "desc" }));
  });

  it("text inputs do not emit until 2+ chars", async () => {
    vi.useFakeTimers();
    try {
      const { wrapper, onUpdate } = mountBar({ dir: "asc" });
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
});
