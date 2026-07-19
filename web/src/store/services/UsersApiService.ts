import { BaseApiService } from "./BaseApiService";
import {
  StatusMessage,
  User as ProtoUserMessage,
  UserList,
  UserPage,
  UserWriteRequest,
} from "../../proto/api";
import type {
  CreateUserPayload,
  StatusResponse,
  UpdateUserPayload,
  User,
  UserPageResponse,
} from "./models";

export class UsersApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async getAllUsers(): Promise<User[]> {
    const response = await this.get("", UserList);
    return response.users;
  }

  async getUserPage(
    cursor?: string,
    limit?: number,
  ): Promise<UserPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/page?${qs}` : "/page";
    return this.get(url, UserPage);
  }

  async getUser(id: number): Promise<User> {
    return this.get(`/${id}`, ProtoUserMessage);
  }

  async createUser(payload: CreateUserPayload): Promise<User> {
    return this.post("", payload, UserWriteRequest, ProtoUserMessage);
  }

  async updateUser(id: number, payload: UpdateUserPayload): Promise<User> {
    return this.put(`/${id}`, payload, UserWriteRequest, ProtoUserMessage);
  }

  async deleteUser(id: number): Promise<StatusResponse> {
    return this.delete(`/${id}`, StatusMessage);
  }
}

export const usersApiService = new UsersApiService();
