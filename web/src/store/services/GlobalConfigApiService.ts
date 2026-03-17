import { BaseApiService } from "./BaseApiService";
import {
  GlobalTournamentConfigList,
  SetTournamentIdsRequest,
  StatusMessage,
} from "../../proto/api";
import type {
  GlobalTournamentConfig,
  SetDeviceTournamentsPayload,
  StatusResponse,
} from "./models";

export class GlobalConfigApiService extends BaseApiService {
  constructor() {
    super("/global-tournament-config");
  }

  async getGlobalConfig(): Promise<GlobalTournamentConfig[]> {
    return (await this.get("", GlobalTournamentConfigList)).configs;
  }

  async removeGlobalConfig(tournamentId: number): Promise<StatusResponse> {
    return this.delete(`/${tournamentId}`, StatusMessage);
  }

  async setGlobalConfig(
    payload: SetDeviceTournamentsPayload,
  ): Promise<GlobalTournamentConfig[]> {
    return (
      await this.post(
        "",
        payload,
        SetTournamentIdsRequest,
        GlobalTournamentConfigList,
      )
    ).configs;
  }
}

export const globalConfigApiService = new GlobalConfigApiService();
