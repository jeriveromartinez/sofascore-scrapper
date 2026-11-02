import { BaseApiService } from "./BaseApiService";
import { EventPage } from "../../proto/api";
import { authJsonAxios } from "./axiosAuth";
import { readAuthStorage } from "../authStorage";
import type {
  EventPageResponse,
  EventsPageFilters,
} from "./models";

export class EventsApiService extends BaseApiService {
  constructor() {
    super("/events");
  }

  /**
   * Fetch the sport categories the events table holds.
   *
   * The endpoint answers JSON, not protobuf: the category list is a
   * plain string array and the catalog endpoints under /scraper-leagues
   * follow the same convention.
   */
  async getSports(signal?: AbortSignal): Promise<string[]> {
    return this.fetchStringList("/sports", signal);
  }

  /**
   * Fetch the league names the events table points at.
   *
   * The endpoint answers JSON, like the category list above.
   */
  async getLeagues(signal?: AbortSignal): Promise<string[]> {
    return this.fetchStringList("/leagues", signal);
  }

  /**
   * Read a list of plain strings from a JSON endpoint of this service.
   *
   * The request accepts every status so the error body can be read: the
   * server answers `{"error": "..."}`, and axios' own message would hide
   * it behind a generic "Request failed with status code 500".
   */
  private async fetchStringList(
    path: string,
    signal?: AbortSignal,
  ): Promise<string[]> {
    const token = readAuthStorage().user?.token ?? "";
    const res = await authJsonAxios.get<{ data?: unknown; error?: string }>(
      `${this.pathApi}${path}`,
      {
        headers: { Authorization: `Bearer ${token}` },
        signal,
        validateStatus: () => true,
      },
    );
    if (res.status >= 400) {
      throw new Error(res.data?.error || `HTTP ${res.status}`);
    }
    const list = Array.isArray(res.data?.data) ? res.data.data : [];
    return list.filter(
      (value): value is string => typeof value === "string" && value !== "",
    );
  }

  async getEventPage(
    cursor?: string,
    limit?: number,
    filters: EventsPageFilters = {},
    signal?: AbortSignal,
  ): Promise<EventPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    if (filters.direction) params.set("direction", filters.direction);
    if (filters.from) params.set("from", filters.from);
    if (filters.tz) params.set("tz", filters.tz);
    if (filters.sport) params.set("sport", filters.sport);
    if (filters.status) params.set("status", filters.status);
    if (filters.league) params.set("league", filters.league);
    if (filters.team) params.set("team", filters.team);
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, EventPage, signal);
  }
}

export const eventsApiService = new EventsApiService();