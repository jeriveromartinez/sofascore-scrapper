import { BaseApiService } from "./BaseApiService";
import type { SofaScoreEvent } from "./models";

export class CurrentEventsApiService extends BaseApiService {
  constructor() {
    super("/api/v1");
  }

  async getCurrentEvents(limit: number = 6): Promise<SofaScoreEvent[]> {
    return this.get<SofaScoreEvent[]>(`/current-events?limit=${limit}`);
  }
}

export const currentEventsApiService = new CurrentEventsApiService();
