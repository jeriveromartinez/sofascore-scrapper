import { BaseApiService } from "./BaseApiService";
import {
  StatusMessage,
  Tournament as ProtoTournamentMessage,
  TournamentList,
  TournamentPage,
  TournamentRequest,
} from "../../proto/api";
import type {
  Tournament,
  CreateTournamentPayload,
  UpdateTournamentPayload,
  StatusResponse,
  TournamentPageResponse,
} from "./models";

export class TournamentsApiService extends BaseApiService {
  constructor() {
    super("/tournaments");
  }

  async getAllTournaments(): Promise<Tournament[]> {
    return (await this.get("", TournamentList)).tournaments;
  }

  async getTournamentPage(
    cursor?: string,
    limit?: number,
  ): Promise<TournamentPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, TournamentPage);
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
