import { BaseApiService } from "./BaseApiService";
import type {
  CreatePlaybackPayload,
  PlaybackLog,
  PlaybackUpdateMethod,
  StatusResponse,
  UpdatePlaybackPayload,
} from "./models";

export class PlaybackApiService extends BaseApiService {
  constructor() {
    super("/playback");
  }

  async createPlayback(payload: CreatePlaybackPayload): Promise<PlaybackLog> {
    return this.post<PlaybackLog, CreatePlaybackPayload>("", payload);
  }

  async updatePlayback(
    id: number,
    payload: UpdatePlaybackPayload,
    method: PlaybackUpdateMethod = "PUT",
  ): Promise<StatusResponse> {
    if (method === "PATCH") {
      return this.patch<StatusResponse, UpdatePlaybackPayload>(
        `/${id}`,
        payload,
      );
    }

    return this.put<StatusResponse, UpdatePlaybackPayload>(`/${id}`, payload);
  }
}

export const playbackApiService = new PlaybackApiService();
