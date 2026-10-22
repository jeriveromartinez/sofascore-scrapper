import { BaseApiService } from "./BaseApiService";
import {
  DeviceList,
  DevicePage,
} from "../../proto/api";
import type { DeviceResponse, DevicePageResponse } from "./models";

export class DevicesApiService extends BaseApiService {
  constructor() {
    super("/devices");
  }

  async getDevicePage(
    cursor?: string,
    limit?: number,
    signal?: AbortSignal,
  ): Promise<DevicePageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, DevicePage, signal);
  }

  async getAllDevices(): Promise<DeviceResponse> {
    return this.get<DeviceResponse>("/all", DeviceList);
  }
}

export const devicesApiService = new DevicesApiService();
