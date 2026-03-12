import { BaseApiService } from "./BaseApiService";
import type { Device, DeviceResponse, RegisterDevicePayload } from "./models";

export class DevicesApiService extends BaseApiService {
  constructor() {
    super("/devices");
  }

  async registerDevice(payload: RegisterDevicePayload): Promise<Device> {
    return this.post<Device, RegisterDevicePayload>("", payload);
  }

  async getDevices({ page, limit }: { page: number; limit: number }): Promise<DeviceResponse> {
    return this.get<DeviceResponse>(`?page=${page}&limit=${limit}`);
  }
}

export const devicesApiService = new DevicesApiService();
