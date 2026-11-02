import { describe, it, expect, vi } from "vitest";
import { EventsApiService } from "./EventsApiService";
import { authJsonAxios } from "./axiosAuth";
import type { EventsPageFilters } from "./models/apiModels";

vi.mock("./axiosAuth", () => ({ authJsonAxios: { get: vi.fn() } }));

vi.mock("../authStorage", () => ({
  readAuthStorage: () => ({ user: { token: "tok" } }),
  writeAuthStorage: vi.fn(),
  clearAuthStorage: vi.fn(),
}));

describe("EventsApiService.getSports", () => {
  it("asks the server for the categories and returns them", async () => {
    const get = authJsonAxios.get as unknown as ReturnType<typeof vi.fn>;
    get.mockResolvedValue({ data: { data: ["football", "cricket"] } });

    const svc = new EventsApiService();

    expect(await svc.getSports()).toEqual(["football", "cricket"]);
    expect(get).toHaveBeenCalledWith(
      "/events/sports",
      expect.objectContaining({
        headers: { Authorization: "Bearer tok" },
      }),
    );
  });

  it("drops the entries that are not category names", async () => {
    const get = authJsonAxios.get as unknown as ReturnType<typeof vi.fn>;
    get.mockResolvedValue({ data: { data: ["football", 7, "", null] } });

    const svc = new EventsApiService();

    expect(await svc.getSports()).toEqual(["football"]);
  });
});

describe("EventsApiService.getLeagues", () => {
  it("asks the server for the league names and returns them", async () => {
    const get = authJsonAxios.get as unknown as ReturnType<typeof vi.fn>;
    get.mockResolvedValue({ data: { data: ["Premier League", "Liga MX"] } });

    const svc = new EventsApiService();

    expect(await svc.getLeagues()).toEqual(["Premier League", "Liga MX"]);
    expect(get).toHaveBeenCalledWith(
      "/events/leagues",
      expect.objectContaining({
        headers: { Authorization: "Bearer tok" },
      }),
    );
  });

  it("drops the entries that are not league names", async () => {
    const get = authJsonAxios.get as unknown as ReturnType<typeof vi.fn>;
    get.mockResolvedValue({ data: { data: ["Liga MX", 7, "", null] } });

    const svc = new EventsApiService();

    expect(await svc.getLeagues()).toEqual(["Liga MX"]);
  });
});

describe("EventsApiService.getEventPage", () => {
  it("forwards direction=desc to the request URL when filters.direction=desc", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10, { direction: "desc" });
    expect(captured[0]).toContain("direction=desc");
    expect(captured[0]).toContain("limit=10");
  });

  it("omits direction when no filters are provided", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10);
    expect(captured[0]).not.toContain("direction=");
  });
});

describe("EventsApiService.getEventPage filter serialization", () => {
  it("serializes direction/from/tz/sport/status/league/team into query string", async () => {
    let capturedUrl = "";
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url) => {
      capturedUrl = url;
      return { data: [], page: undefined };
    };
    const filters: EventsPageFilters = {
      direction: "desc",
      from: "2026-08-26",
      tz: "America/Santo_Domingo",
      sport: "football",
      status: "notstarted",
      league: "Primera",
      team: "Barcelona",
    };
    await svc.getEventPage(undefined, 20, filters);
    expect(capturedUrl).toContain("direction=desc");
    expect(capturedUrl).toContain("from=2026-08-26");
    expect(capturedUrl).toContain("tz=America%2FSanto_Domingo");
    expect(capturedUrl).toContain("sport=football");
    expect(capturedUrl).toContain("status=notstarted");
    expect(capturedUrl).toContain("league=Primera");
    expect(capturedUrl).toContain("team=Barcelona");
  });

  it("omits filters that are empty strings", async () => {
    let capturedUrl = "";
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url) => {
      capturedUrl = url;
      return { data: [], page: undefined };
    };
    await svc.getEventPage(undefined, 20, {
      direction: "asc",
      from: "2026-08-26",
      tz: "UTC",
      sport: "",
      status: "",
      league: "",
      team: "",
    });
    expect(capturedUrl).not.toContain("sport=");
    expect(capturedUrl).not.toContain("status=");
    expect(capturedUrl).not.toContain("league=");
    expect(capturedUrl).not.toContain("team=");
  });
});
