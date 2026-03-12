import { BaseApiService } from "./BaseApiService";
import type {
  DeviceTournament,
  AssignTournamentPayload,
  SetDeviceTournamentsPayload,
  StatusResponse,
} from "./models";

export class DeviceTournamentsApiService extends BaseApiService {
  constructor() {
    super("/api/v1");
  }

  async getAllDeviceTournaments(): Promise<DeviceTournament[]> {
    return this.get<DeviceTournament[]>("/device-tournaments");
  }

  async getDeviceTournaments(deviceId: number): Promise<DeviceTournament[]> {
    return this.get<DeviceTournament[]>(`/device-tournaments/${deviceId}`);
  }

  async assignTournamentToDevice(payload: AssignTournamentPayload): Promise<DeviceTournament> {
    return this.post<DeviceTournament, AssignTournamentPayload>("/device-tournaments", payload);
  }

  async removeTournamentFromDevice(payload: AssignTournamentPayload): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/device-tournaments?device_id=${payload.device_id}&tournament_id=${payload.tournament_id}`);
  }

  async setDeviceTournaments(deviceId: number, payload: SetDeviceTournamentsPayload): Promise<StatusResponse> {
    return this.put<StatusResponse, SetDeviceTournamentsPayload>(`/device-tournaments/${deviceId}`, payload);
  }
}

export const deviceTournamentsApiService = new DeviceTournamentsApiService();
