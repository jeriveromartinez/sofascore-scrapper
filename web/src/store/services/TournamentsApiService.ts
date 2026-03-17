import { BaseApiService } from "./BaseApiService";
import {
  StatusMessage,
  Tournament as ProtoTournamentMessage,
  TournamentList,
  TournamentRequest,
} from "../../proto/api";
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
    return (await this.get("", TournamentList)).tournaments;
  }

  async getTournament(id: number): Promise<Tournament> {
    return this.get(`/${id}`, ProtoTournamentMessage);
  }

  async createTournament(
    payload: CreateTournamentPayload,
  ): Promise<Tournament> {
    return this.post("", payload, TournamentRequest, ProtoTournamentMessage);
  }

  async updateTournament(
    id: number,
    payload: UpdateTournamentPayload,
  ): Promise<Tournament> {
    return this.put(
      `/${id}`,
      payload,
      TournamentRequest,
      ProtoTournamentMessage,
    );
  }

  async deleteTournament(id: number): Promise<StatusResponse> {
    return this.delete(`/${id}`, StatusMessage);
  }
}

export const tournamentsApiService = new TournamentsApiService();
