export interface UserAuthModel {
  id: number;
  email: string;
  token: string;
  refreshToken: string;
}

export interface UserAuthPayload {
  email: string;
  password: string;
}
