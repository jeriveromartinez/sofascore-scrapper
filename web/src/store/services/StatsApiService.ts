import { BaseApiService } from "./BaseApiService";
import { TopEventsResponse } from "../../proto/api";
import type { EventStats } from "./models";

export class StatsApiService extends BaseApiService {
  constructor() {
    super("/stats");
  }

  async getTopEvents(limit?: number): Promise<EventStats[]> {
    const suffix = limit ? `?limit=${encodeURIComponent(String(limit))}` : "";
    return (await this.get(`/top-events${suffix}`, TopEventsResponse)).stats;
  }
}

export const statsApiService = new StatsApiService();
