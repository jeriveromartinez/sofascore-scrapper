import { BaseApiService } from "./BaseApiService";
import {
  LogPlaybackRequest,
  PlaybackLog as ProtoPlaybackLogMessage,
  StatusResponse as ProtoStatusResponseMessage,
  UpdatePlaybackRequest,
} from "../../proto/api";
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
    return this.post("", payload, LogPlaybackRequest, ProtoPlaybackLogMessage);
  }

  async updatePlayback(
    id: number,
    payload: UpdatePlaybackPayload,
    method: PlaybackUpdateMethod = "PUT",
  ): Promise<StatusResponse> {
    if (method === "PATCH") {
      return this.patch(
        `/${id}`,
        payload,
        UpdatePlaybackRequest,
        ProtoStatusResponseMessage,
      );
    }

    return this.put(
      `/${id}`,
      payload,
      UpdatePlaybackRequest,
      ProtoStatusResponseMessage,
    );
  }
}

export const playbackApiService = new PlaybackApiService();
