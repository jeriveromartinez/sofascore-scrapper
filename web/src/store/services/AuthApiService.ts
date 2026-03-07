import { BaseApiService } from "./BaseApiService";
import { type UserAuthModel, type UserAuthPayload } from "./models";

export class AuthApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async login(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>("/login", {
      email,
      password,
    });
    return data;
  }

  async register(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthPayload>("/register", {
      email,
      password,
    });
    return data;
  }
}

export const authApiService = new AuthApiService();
export default authApiService;
