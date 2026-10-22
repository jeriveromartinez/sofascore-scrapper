import { BaseApiService } from "./BaseApiService";
import {
  DeviceTournamentList,
  SetTournamentIdsRequest,
  StatusMessage,
} from "../../proto/api";
import type {
  DeviceTournament,
  SetDeviceTournamentsPayload,
  StatusResponse,
} from "./models";

export class DeviceTournamentsApiService extends BaseApiService {
  constructor() {
    super("/device-tournaments");
  }

  async getDeviceTournaments(deviceId: number): Promise<DeviceTournament[]> {
    var resp = (await this.get(`/${deviceId}`, DeviceTournamentList)) ?? {
      deviceTournaments: [],
    };
    return resp.deviceTournaments;
  }

  async setDeviceTournaments(
    deviceId: number,
    payload: SetDeviceTournamentsPayload,
  ): Promise<StatusResponse> {
    return this.put(
      `/${deviceId}`,
      payload,
      SetTournamentIdsRequest,
      StatusMessage,
    );
  }
}

export const deviceTournamentsApiService = new DeviceTournamentsApiService();
