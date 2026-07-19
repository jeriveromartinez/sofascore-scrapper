import { BaseApiService } from "./BaseApiService";
import {
  AssignTournamentRequest,
  DeviceTournament as ProtoDeviceTournamentMessage,
  DeviceTournamentList,
  DeviceTournamentPage,
  SetTournamentIdsRequest,
  StatusMessage,
} from "../../proto/api";
import type {
  DeviceTournament,
  DeviceTournamentPageResponse,
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

  async getDeviceTournamentPage(
    cursor?: string,
    limit?: number,
  ): Promise<DeviceTournamentPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, DeviceTournamentPage);
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
