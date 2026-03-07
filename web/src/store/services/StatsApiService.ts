import { BaseApiService } from "./BaseApiService";
import type { EventStats } from "./models";

export class StatsApiService extends BaseApiService {
  constructor() {
    super("/stats");
  }

  async getTopEvents(limit?: number): Promise<EventStats[]> {
    const suffix = limit ? `?limit=${encodeURIComponent(String(limit))}` : "";
    return this.get<EventStats[]>(`/top-events${suffix}`);
  }
}

export const statsApiService = new StatsApiService();
