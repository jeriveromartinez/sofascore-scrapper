import { BaseApiService } from "./BaseApiService";
import type {
  DeviceTournament,
  AssignTournamentPayload,
  SetDeviceTournamentsPayload,
  StatusResponse,
} from "./models";

export class DeviceTournamentsApiService extends BaseApiService {
  constructor() {
    super("device-tournaments");
  }

  async getAllDeviceTournaments(): Promise<DeviceTournament[]> {
    return this.get<DeviceTournament[]>("");
  }

  async getDeviceTournaments(deviceId: number): Promise<DeviceTournament[]> {
    return this.get<DeviceTournament[]>(`/${deviceId}`);
  }

  async assignTournamentToDevice(payload: AssignTournamentPayload): Promise<DeviceTournament> {
    return this.post<DeviceTournament, AssignTournamentPayload>("", payload);
  }

  async removeTournamentFromDevice(payload: AssignTournamentPayload): Promise<StatusResponse> {
    return this.deleteWithBody<StatusResponse, AssignTournamentPayload>("", payload);
  }

  async setDeviceTournaments(deviceId: number, payload: SetDeviceTournamentsPayload): Promise<StatusResponse> {
    return this.put<StatusResponse, SetDeviceTournamentsPayload>(`/${deviceId}`, payload);
  }
}

export const deviceTournamentsApiService = new DeviceTournamentsApiService();
