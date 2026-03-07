import { BaseApiService } from "./BaseApiService";
import type { EventsQuery, EventsResponse } from "./models";

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
    return this.get<EventsResponse>(toQueryString(query));
  }
}

export const eventsApiService = new EventsApiService();
