import { BaseApiService } from "./BaseApiService";
import {
  AssignTournamentRequest,
  DeviceTournament as ProtoDeviceTournamentMessage,
  DeviceTournamentList,
  SetTournamentIdsRequest,
  StatusMessage,
} from "../../proto/api";
import type {
  DeviceTournament,
  AssignTournamentPayload,
  SetDeviceTournamentsPayload,
  StatusResponse,
} from "./models";

export class DeviceTournamentsApiService extends BaseApiService {
  constructor() {
    super("/device-tournaments");
  }

  async getAllDeviceTournaments(): Promise<DeviceTournament[]> {
    return (await this.get("", DeviceTournamentList)).deviceTournaments;
  }

  async getDeviceTournaments(deviceId: number): Promise<DeviceTournament[]> {
    var resp = (await this.get(`/${deviceId}`, DeviceTournamentList)) ?? {
      deviceTournaments: [],
    };
    return resp.deviceTournaments;
  }

  async assignTournamentToDevice(
    payload: AssignTournamentPayload,
  ): Promise<DeviceTournament> {
    return this.post(
      "",
      payload,
      AssignTournamentRequest,
      ProtoDeviceTournamentMessage,
    );
  }

  async removeTournamentFromDevice(
    payload: AssignTournamentPayload,
  ): Promise<StatusResponse> {
    return this.deleteWithBody(
      "",
      payload,
      AssignTournamentRequest,
      StatusMessage,
    );
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
