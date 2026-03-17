import { BaseApiService } from "./BaseApiService";
import { EventsList } from "../../proto/api";
import type { SofaScoreEvent } from "./models";

export class CurrentEventsApiService extends BaseApiService {
  constructor() {
    super("");
  }

  async getCurrentEvents(limit: number = 6): Promise<SofaScoreEvent[]> {
    return (await this.get(`/current-events?limit=${limit}`, EventsList)).data;
  }
}

export const currentEventsApiService = new CurrentEventsApiService();
