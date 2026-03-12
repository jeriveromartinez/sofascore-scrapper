import { BaseApiService } from "./BaseApiService";
import type { GlobalTournamentConfig, AddGlobalConfigPayload, SetGlobalConfigPayload, StatusResponse } from "./models";

export class GlobalConfigApiService extends BaseApiService {
  constructor() {
    super("global-tournament-config");
  }

  async getGlobalConfig(): Promise<GlobalTournamentConfig[]> {
    return this.get<GlobalTournamentConfig[]>("");
  }

  async addGlobalConfig(payload: AddGlobalConfigPayload): Promise<GlobalTournamentConfig> {
    return this.post<GlobalTournamentConfig, AddGlobalConfigPayload>("", payload);
  }

  async removeGlobalConfig(tournamentId: number): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/${tournamentId}`);
  }

  async setGlobalConfig(payload: SetGlobalConfigPayload): Promise<StatusResponse> {
    return this.put<StatusResponse, SetGlobalConfigPayload>("", payload);
  }
}

export const globalConfigApiService = new GlobalConfigApiService();
