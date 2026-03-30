import { BaseApiService } from "./BaseApiService";
import { PlaybackLogList } from "../../proto/api";

export class PlaybackApiService extends BaseApiService {
  constructor() {
    super("/playback");
  }

  async getPlayingNow(page: number, limit: number): Promise<PlaybackLogList> {
    return await this.get(`?page=${page}&limit=${limit}`, PlaybackLogList);
  }
}

export const playbackApiService = new PlaybackApiService();
