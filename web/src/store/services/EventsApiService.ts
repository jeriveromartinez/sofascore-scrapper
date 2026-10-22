import { BaseApiService } from "./BaseApiService";
import { EventPage } from "../../proto/api";
import type {
  EventPageResponse,
  EventsPageFilters,
} from "./models";

export class EventsApiService extends BaseApiService {
  constructor() {
    super("/events");
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