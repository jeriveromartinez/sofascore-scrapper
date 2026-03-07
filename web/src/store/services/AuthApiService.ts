import { BaseApiService } from "./BaseApiService";
import { UserAuthModel } from "./models";

export class AuthApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async login(email: string, password: string): Promise<UserAuthModel> {
    console.log("Attempting login with", email, password);
    const data = await this.post<UserAuthModel, UserAuthModel>("/login", {
      email,
      password,
    });
    return data;
  }

  async register(email: string, password: string): Promise<UserAuthModel> {
    const data = await this.post<UserAuthModel, UserAuthModel>("/register", {
      email,
      password,
    });
    return data;
  }
}

export const authApiService = new AuthApiService();
export default authApiService;
