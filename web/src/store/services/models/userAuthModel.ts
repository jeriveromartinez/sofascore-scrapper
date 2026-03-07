export interface UserAuthModel {
  id: number;
  email: string;
  token: string;
}

export interface UserAuthPayload {
  email: string;
  password: string;
}
