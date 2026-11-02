import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { toast } from "vue3-toastify";
import EventsFilterBar from "./EventsFilterBar.vue";
import type { EventsPageFilters } from "../store/services/models/apiModels";

const { getSports, getLeagues } = vi.hoisted(() => ({
  getSports: vi.fn<() => Promise<string[]>>(),
  getLeagues: vi.fn<() => Promise<string[]>>(),
}));

vi.mock("../store/services", () => ({
  eventsApiService: { getSports, getLeagues },
}));

vi.mock("vue3-toastify", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function mountBar(filters: EventsPageFilters) {
  const onUpdate = vi.fn();
  const wrapper = mount(EventsFilterBar, {
    props: { modelValue: filters, "onUpdate:modelValue": onUpdate },
  });
  return { wrapper, onUpdate };
}

function sportValues(wrapper: VueWrapper): string[] {
  return wrapper
    .findAll('select[name="sport"] option')
    .map((option) => (option.element as HTMLOptionElement).value);
}

beforeEach(() => {
  getSports.mockReset();
  getSports.mockResolvedValue([]);
  getLeagues.mockReset();
  getLeagues.mockResolvedValue([]);
  vi.mocked(toast.error).mockReset();
});

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

  it("offers every category the database holds", async () => {
    getSports.mockResolvedValue(["football", "cricket"]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    expect(sportValues(wrapper)).toEqual(["", "football", "cricket"]);
  });

  it("keeps the selected category even when the database does not list it", async () => {
    getSports.mockResolvedValue(["cricket"]);
    const { wrapper } = mountBar({ direction: "asc", sport: "chess" });
    await flushPromises();

    expect(sportValues(wrapper)).toEqual(["", "chess", "cricket"]);
  });

  it("labels an unknown category with its raw slug", async () => {
    getSports.mockResolvedValue(["cricket"]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    const labels = wrapper
      .findAll('select[name="sport"] option')
      .map((option) => option.text());
    expect(labels).toContain("cricket");
  });

  it("falls back to the canonical categories when the database lists none", async () => {
    getSports.mockResolvedValue([]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    expect(sportValues(wrapper)).toEqual([
      "",
      "football",
      "basketball",
      "tennis",
      "ice-hockey",
      "handball",
      "american-football",
      "baseball",
      "volleyball",
      "rugby",
    ]);
  });

  it("falls back to the canonical categories when the request fails", async () => {
    getSports.mockRejectedValue(new Error("no se pudieron cargar las categorías"));
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    expect(sportValues(wrapper)).toEqual([
      "",
      "football",
      "basketball",
      "tennis",
      "ice-hockey",
      "handball",
      "american-football",
      "baseball",
      "volleyball",
      "rugby",
    ]);
    expect(toast.error).toHaveBeenCalledWith("no se pudieron cargar las categorías");
  });

  it("does not offer the legacy hockey slug", async () => {
    getSports.mockResolvedValue(["football", "ice-hockey"]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    const values = sportValues(wrapper);

    expect(values).not.toContain("hockey");
    expect(values).toContain("ice-hockey");
  });

  it("suggests the leagues the database holds", async () => {
    getLeagues.mockResolvedValue(["Premier League", "Liga MX"]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    const options = wrapper
      .findAll("#events-league-options option")
      .map((option) => (option.element as HTMLOptionElement).value);

    expect(options).toEqual(["Premier League", "Liga MX"]);
    expect(
      wrapper.find('input[name="league"]').attributes("list"),
    ).toBe("events-league-options");
  });

  it("leaves the league suggestions empty when the database lists none", async () => {
    getLeagues.mockResolvedValue([]);
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    expect(wrapper.findAll("#events-league-options option")).toHaveLength(0);
  });

  it("reports the failure and keeps the league suggestions empty", async () => {
    getLeagues.mockRejectedValue(new Error("no se pudieron cargar las ligas"));
    const { wrapper } = mountBar({ direction: "asc" });
    await flushPromises();

    expect(wrapper.findAll("#events-league-options option")).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith("no se pudieron cargar las ligas");
  });

  it("emits the selected sport", async () => {
    const { wrapper, onUpdate } = mountBar({ direction: "asc" });

    await wrapper.find('select[name="sport"]').setValue("handball");

    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ sport: "handball" }));
  });

  it("shows how many filters are active", () => {
    const { wrapper } = mountBar({
      direction: "asc",
      sport: "football",
      status: "inprogress",
      league: "Premier",
    });

    expect(wrapper.find('[data-testid="active-filters"]').text()).toContain("3");
  });

  it("clears every filter but keeps the date, timezone and direction", async () => {
    const { wrapper, onUpdate } = mountBar({
      direction: "desc",
      from: "2026-10-31",
      tz: "Europe/Madrid",
      sport: "football",
      status: "finished",
      league: "Premier",
      team: "Arsenal",
    });

    await wrapper.find('[data-testid="clear-filters"]').trigger("click");

    expect(onUpdate).toHaveBeenCalledWith({
      direction: "desc",
      from: "2026-10-31",
      tz: "Europe/Madrid",
      sport: "",
      status: "",
      league: "",
      team: "",
    });
  });

  it("disables the clear button when no filter is active", () => {
    const { wrapper } = mountBar({ direction: "asc", from: "2026-10-31", tz: "UTC" });

    expect(wrapper.find('[data-testid="clear-filters"]').attributes("disabled")).toBeDefined();
  });
});
