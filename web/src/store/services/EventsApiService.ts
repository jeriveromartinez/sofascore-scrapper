import { BaseApiService } from "./BaseApiService";
import { EventsList, EventPage } from "../../proto/api";
import type {
  EventsQuery,
  EventsResponse,
  EventPageResponse,
  EventsPageFilters,
} from "./models";

function toQueryString(query: EventsQuery): string {
  const params = new URLSearchParams();

  if (query.date) params.set("date", query.date);
  if (query.sport) params.set("sport", query.sport);
  if (query.page) params.set("page", String(query.page));
  if (query.limit) params.set("limit", String(query.limit));

  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
}

export class EventsApiService extends BaseApiService {
  constructor() {
    super("/events");
  }

  async getEvents(query: EventsQuery = {}): Promise<EventsResponse> {
    return this.get(toQueryString(query), EventsList);
  }

  async getEventPage(
    cursor?: string,
    limit?: number,
    filters: EventsPageFilters = {},
  ): Promise<EventPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    if (filters.dir) params.set("direction", filters.dir);
    if (filters.from) params.set("from", filters.from);
    if (filters.tz) params.set("tz", filters.tz);
    if (filters.sport) params.set("sport", filters.sport);
    if (filters.status) params.set("status", filters.status);
    if (filters.league) params.set("league", filters.league);
    if (filters.team) params.set("team", filters.team);
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, EventPage);
  }
}

export const eventsApiService = new EventsApiService();