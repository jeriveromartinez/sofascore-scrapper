import { BaseApiService } from "./BaseApiService";
import {
  Device as ProtoDeviceMessage,
  DeviceList,
  DeviceRegisterRequest,
} from "../../proto/api";
import type { Device, DeviceResponse, RegisterDevicePayload } from "./models";

export class DevicesApiService extends BaseApiService {
  constructor() {
    super("/devices");
  }

  async registerDevice(payload: RegisterDevicePayload): Promise<Device> {
    return this.post<Device, RegisterDevicePayload>(
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

  async getAllDevices(): Promise<DeviceResponse> {
    return this.get<DeviceResponse>("/all", DeviceList);
  }
}

export const devicesApiService = new DevicesApiService();
