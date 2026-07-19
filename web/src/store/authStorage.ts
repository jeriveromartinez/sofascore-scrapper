import { KEY_USER_LOGIN } from "../constants";
import type { UserAuthModel } from "./services/models";

export function readAuthStorage(): {
  user: UserAuthModel | null;
  storage: Storage | null;
} {
  for (const storage of [sessionStorage, localStorage]) {
    const raw = storage.getItem(KEY_USER_LOGIN);
    if (!raw) continue;
    try {
      return { user: JSON.parse(raw) as UserAuthModel, storage };
    } catch {
      storage.removeItem(KEY_USER_LOGIN);
    }
  }
  return { user: null, storage: null };
}

export function writeAuthStorage(
  user: UserAuthModel,
  rememberMe: boolean,
): void {
  const raw = JSON.stringify(user);
  if (rememberMe) {
    localStorage.setItem(KEY_USER_LOGIN, raw);
  } else {
    sessionStorage.setItem(KEY_USER_LOGIN, raw);
  }
}

export function clearAuthStorage(): void {
  sessionStorage.removeItem(KEY_USER_LOGIN);
  localStorage.removeItem(KEY_USER_LOGIN);
}
