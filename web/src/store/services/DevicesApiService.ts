import { BaseApiService } from "./BaseApiService";
import type { Device, RegisterDevicePayload } from "./models";

export class DevicesApiService extends BaseApiService {
  constructor() {
    super("/devices");
  }

  async registerDevice(payload: RegisterDevicePayload): Promise<Device> {
    return this.post<Device, RegisterDevicePayload>("", payload);
  }
}

export const devicesApiService = new DevicesApiService();
