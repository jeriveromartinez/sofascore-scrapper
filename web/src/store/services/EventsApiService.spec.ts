import { describe, it, expect } from "vitest";
import { EventsApiService } from "./EventsApiService";

describe("EventsApiService.getEventPage", () => {
  it("forwards direction=desc to the request URL", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10, "desc");
    expect(captured[0]).toContain("direction=desc");
    expect(captured[0]).toContain("limit=10");
  });

  it("defaults to direction=asc when not provided", async () => {
    const captured: string[] = [];
    const svc = new EventsApiService();
    (svc as unknown as { get: (url: string) => Promise<unknown> }).get = async (url: string) => {
      captured.push(url);
      return { data: [], page: { nextCursor: "", hasMore: false } };
    };
    await svc.getEventPage(undefined, 10);
    expect(captured[0]).toContain("direction=asc");
  });
});
