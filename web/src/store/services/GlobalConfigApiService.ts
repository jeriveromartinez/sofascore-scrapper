import { BaseApiService } from "./BaseApiService";
import type { GlobalTournamentConfig, SetDeviceTournamentsPayload, StatusResponse } from "./models";

export class GlobalConfigApiService extends BaseApiService {
  constructor() {
    super("global-tournament-config");
  }

  async getGlobalConfig(): Promise<GlobalTournamentConfig[]> {
    return this.get<GlobalTournamentConfig[]>("");
  }

  async removeGlobalConfig(tournamentId: number): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/${tournamentId}`);
  }

  async setGlobalConfig(payload: SetDeviceTournamentsPayload): Promise<StatusResponse> {
    return this.post<StatusResponse, SetDeviceTournamentsPayload>("", payload);
  }
}

export const globalConfigApiService = new GlobalConfigApiService();
