import { BaseApiService } from "./BaseApiService";
import type {
  Tournament,
  CreateTournamentPayload,
  UpdateTournamentPayload,
  StatusResponse,
} from "./models";

export class TournamentsApiService extends BaseApiService {
  constructor() {
    super("/tournaments");
  }

  async getAllTournaments(): Promise<Tournament[]> {
    return this.get<Tournament[]>("");
  }

  async getTournament(id: number): Promise<Tournament> {
    return this.get<Tournament>(`/${id}`);
  }

  async createTournament(payload: CreateTournamentPayload): Promise<Tournament> {
    return this.post<Tournament, CreateTournamentPayload>("", payload);
  }

  async updateTournament(id: number, payload: UpdateTournamentPayload): Promise<Tournament> {
    return this.put<Tournament, UpdateTournamentPayload>(`/${id}`, payload);
  }

  async deleteTournament(id: number): Promise<StatusResponse> {
    return this.delete<StatusResponse>(`/${id}`);
  }
}

export const tournamentsApiService = new TournamentsApiService();
