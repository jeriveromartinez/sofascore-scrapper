import { BaseApiService } from "./BaseApiService";
import { EventsList } from "../../proto/api";
import type { ExternalEvent } from "./models";

export class CurrentEventsApiService extends BaseApiService {
  constructor() {
    super("");
  }

  async getCurrentEvents(limit: number = 6): Promise<ExternalEvent[]> {
    return (await this.get(`/current-events?limit=${limit}`, EventsList)).data;
  }
}

export const currentEventsApiService = new CurrentEventsApiService();
