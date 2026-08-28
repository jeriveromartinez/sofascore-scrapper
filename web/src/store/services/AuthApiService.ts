import { BaseApiService } from "./BaseApiService";
import { AuthRequest, AuthResponse, StatusMessage } from "../../proto/api";
import { type UserAuthModel, type UserAuthPayload } from "./models";
import { KEY_USER_LOGIN } from "../../constants";

export class AuthApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async login(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>(
      "/login",
      { email, password },
      AuthRequest,
      AuthResponse,
    );
    return data;
  }

  async register(email: string, password: string, invitationToken: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>(
      "/register",
      { email, password, invitationToken },
      AuthRequest,
      AuthResponse,
    );
    return data;
  }

  async logout(): Promise<void> {
    const storedUser =
      sessionStorage.getItem(KEY_USER_LOGIN) ??
      localStorage.getItem(KEY_USER_LOGIN) ??
      "{}";

    let refreshToken = "";
    try {
      refreshToken = (JSON.parse(storedUser) as UserAuthModel).refreshToken ?? "";
    } catch {
      refreshToken = "";
    }

    await this.postWithoutBody(
      "/logout",
      StatusMessage,
      refreshToken ? { "X-Refresh-Token": refreshToken } : undefined,
    );
  }
}

export const authApiService = new AuthApiService();
export default authApiService;
