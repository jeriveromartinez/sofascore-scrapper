export class UserAuthModel {
  email: string = "";
  password: string = "";
  readonly token?: string;
}

export default UserAuthModel;
