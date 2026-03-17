import { BaseApiService } from "./BaseApiService";
import { AuthRequest, AuthResponse } from "../../proto/api";
import { type UserAuthModel, type UserAuthPayload } from "./models";

export class AuthApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async login(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>(
      "/login",
      {
        email,
        password,
      },
      AuthRequest,
      AuthResponse,
    );
    return data;
  }

  async register(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>(
      "/register",
      {
        email,
        password,
      },
      AuthRequest,
      AuthResponse,
    );
    return data;
  }
}

export const authApiService = new AuthApiService();
export default authApiService;
