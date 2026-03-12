import { BaseApiService } from "./BaseApiService";
import type {
  Tournament,
  CreateTournamentPayload,
  UpdateTournamentPayload,
  StatusResponse,
} from "./models";

export class TournamentsApiService extends BaseApiService {
  constructor() {
    super("/api/v1");
  }

  async getAllTournaments(): Promise<Tournament[]> {
    return this.get<Tournament[]>("/tournaments");
  }

  async getTournament(id: number): Promise<Tournament> {
    return this.get<Tournament>(`/tournaments/${id}`);
  }

  async createTournament(payload: CreateTournamentPayload): Promise<Tournament> {
    return this.post<Tournament, CreateTournamentPayload>("/tournaments", payload);
  }

  async updateTournament(id: number, payload: UpdateTournamentPayload): Promise<Tournament> {
    return this.put<Tournament, UpdateTournamentPayload>(`/tournaments/${id}`, payload);
  }

  async deleteTournament(id: number): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/tournaments/${id}`);
  }
}

export const tournamentsApiService = new TournamentsApiService();
