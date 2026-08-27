import { describe, it, expect, vi } from "vitest";
import { EventsApiService } from "./EventsApiService";
import type { EventsPageFilters } from "./models/apiModels";

describe("EventsApiService.getEventPage", () => {
  it("forwards dir=desc to the request URL when filters.dir=desc", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10, { dir: "desc" });
    expect(captured[0]).toContain("direction=desc");
    expect(captured[0]).toContain("limit=10");
  });

  it("omits dir when no filters are provided", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10);
    expect(captured[0]).not.toContain("dir=");
  });
});

describe("EventsApiService.getEventPage filter serialization", () => {
  it("serializes dir/from/tz/sport/status/league/team into query string", async () => {
    const svc = new EventsApiService();
    const fetchSpy = vi.spyOn(svc, "get").mockResolvedValue({ data: [], page: undefined });
    const filters: EventsPageFilters = {
      dir: "desc",
      from: "2026-08-26",
      tz: "America/Santo_Domingo",
      sport: "football",
      status: "notstarted",
      league: "Primera",
      team: "Barcelona",
    };
    await svc.getEventPage(undefined, 20, filters);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = fetchSpy.mock.calls[0][0] as string;
    expect(url).toContain("direction=desc");
    expect(url).toContain("from=2026-08-26");
    expect(url).toContain("tz=America%2FSanto_Domingo");
    expect(url).toContain("sport=football");
    expect(url).toContain("status=notstarted");
    expect(url).toContain("league=Primera");
    expect(url).toContain("team=Barcelona");
  });

  it("omits filters that are empty strings", async () => {
    const svc = new EventsApiService();
    const fetchSpy = vi.spyOn(svc, "get").mockResolvedValue({ data: [], page: undefined });
    await svc.getEventPage(undefined, 20, {
      dir: "asc",
      from: "2026-08-26",
      tz: "UTC",
      sport: "",
      status: "",
      league: "",
      team: "",
    });
    const url = fetchSpy.mock.calls[0][0] as string;
    expect(url).not.toContain("sport=");
    expect(url).not.toContain("status=");
    expect(url).not.toContain("league=");
    expect(url).not.toContain("team=");
  });
});
