import { BaseApiService } from "./BaseApiService";
import {
  Domain as ProtoDomainMessage,
  DomainList,
  DomainPage,
  DomainRequest,
  StatusMessage,
} from "../../proto/api";
import type {
  CreateDomainPayload,
  Domain,
  StatusResponse,
  UpdateDomainPayload,
  DomainPageResponse,
} from "./models";

export class DomainsApiService extends BaseApiService {
  constructor() {
    super("/domains");
  }

  async getAllDomains(): Promise<Domain[]> {
    const response = await this.get("", DomainList);
    return response?.domains ?? [];
  }

  async getDomainPage(
    cursor?: string,
    limit?: number,
  ): Promise<DomainPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, DomainPage);
  }

  async getDomain(id: number): Promise<Domain> {
    return this.get(`/${id}`, ProtoDomainMessage);
  }

  async createDomain(payload: CreateDomainPayload): Promise<Domain> {
    return this.post("", payload, DomainRequest, ProtoDomainMessage);
  }

  async updateDomain(
    id: number,
    payload: UpdateDomainPayload,
  ): Promise<Domain> {
    return this.put(`/${id}`, payload, DomainRequest, ProtoDomainMessage);
  }

  async deleteDomain(id: number): Promise<StatusResponse> {
    return this.delete(`/${id}`, StatusMessage);
  }
}

export const domainsApiService = new DomainsApiService();
