import { BaseApiService } from "./BaseApiService";
import {
  GlobalTournamentConfigList,
  SetTournamentIdsRequest,
} from "../../proto/api";
import type {
  GlobalTournamentConfig,
  SetDeviceTournamentsPayload,
} from "./models";

export class GlobalConfigApiService extends BaseApiService {
  constructor() {
    super("/global-tournament-config");
  }

  async getGlobalConfig(): Promise<GlobalTournamentConfig[]> {
    const resp = (await this.get("", GlobalTournamentConfigList)) ?? {
      configs: [],
    };
    return resp.configs ?? [];
  }

  async setGlobalConfig(
    payload: SetDeviceTournamentsPayload,
  ): Promise<GlobalTournamentConfig[]> {
    var resp = (await this.post(
      "",
      payload,
      SetTournamentIdsRequest,
      GlobalTournamentConfigList,
    )) ?? { configs: [] };
    return resp.configs ?? [];
  }
}

export const globalConfigApiService = new GlobalConfigApiService();
