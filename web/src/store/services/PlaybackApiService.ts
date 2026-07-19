import { BaseApiService } from "./BaseApiService";
import { PlaybackLogList, PlaybackPage } from "../../proto/api";
import type { PlaybackPageResponse } from "./models";

export class PlaybackApiService extends BaseApiService {
  constructor() {
    super("/playback");
  }

  async getPlayingNow(page: number, limit: number): Promise<PlaybackLogList> {
    return await this.get(`?page=${page}&limit=${limit}`, PlaybackLogList);
  }

  async getPlaybackPage(
    cursor?: string,
    limit?: number,
  ): Promise<PlaybackPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, PlaybackPage);
  }
}

export const playbackApiService = new PlaybackApiService();
