import { BaseApiService } from "./BaseApiService";
import type {
  GlobalTournamentConfig,
  AddGlobalConfigPayload,
  SetGlobalConfigPayload,
  StatusResponse,
} from "./models";

export class GlobalConfigApiService extends BaseApiService {
  constructor() {
    super("/api/v1");
  }

  async getGlobalConfig(): Promise<GlobalTournamentConfig[]> {
    return this.get<GlobalTournamentConfig[]>("/global-tournament-config");
  }

  async addGlobalConfig(payload: AddGlobalConfigPayload): Promise<GlobalTournamentConfig> {
    return this.post<GlobalTournamentConfig, AddGlobalConfigPayload>("/global-tournament-config", payload);
  }

  async removeGlobalConfig(tournamentId: number): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/global-tournament-config/${tournamentId}`);
  }

  async setGlobalConfig(payload: SetGlobalConfigPayload): Promise<StatusResponse> {
    return this.put<StatusResponse, SetGlobalConfigPayload>("/global-tournament-config", payload);
  }
}

export const globalConfigApiService = new GlobalConfigApiService();
