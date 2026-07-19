import { BaseApiService } from "./BaseApiService";
import {
  Device as ProtoDeviceMessage,
  DeviceList,
  DevicePage,
  DeviceRegisterRequest,
} from "../../proto/api";
import type { Device, DeviceResponse, DevicePageResponse, RegisterDevicePayload } from "./models";

export class DevicesApiService extends BaseApiService {
  constructor() {
    super("/devices");
  }

  async updateDevice(payload: RegisterDevicePayload): Promise<Device> {
    return this.put<Device, RegisterDevicePayload>(
      "",
      payload,
      DeviceRegisterRequest,
      ProtoDeviceMessage,
    );
  }

  async getDevices({
    page,
    limit,
  }: {
    page: number;
    limit: number;
  }): Promise<DeviceResponse> {
    return this.get<DeviceResponse>(`?page=${page}&limit=${limit}`, DeviceList);
  }

  async getDevicePage(
    cursor?: string,
    limit?: number,
  ): Promise<DevicePageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, DevicePage);
  }

  async getAllDevices(): Promise<DeviceResponse> {
    return this.get<DeviceResponse>("/all", DeviceList);
  }
}

export const devicesApiService = new DevicesApiService();
