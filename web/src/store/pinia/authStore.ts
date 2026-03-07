import { KEY_USER_LOGIN } from "../../constants";
import type { UserAuthModel } from "../services/models";
import { defineStore } from "pinia";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    userData: JSON.parse(
      sessionStorage.getItem(KEY_USER_LOGIN) ??
        localStorage.getItem(KEY_USER_LOGIN) ??
        "{}",
    ) as Partial<UserAuthModel>,
  }),
  actions: {
    setUser(userData: UserAuthModel, rememberMe: boolean) {
      this.userData = userData;
      if (rememberMe)
        localStorage.setItem(KEY_USER_LOGIN, JSON.stringify(userData));
      else sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(userData));
    },
    clearUser() {
      this.userData = {};
      sessionStorage.removeItem(KEY_USER_LOGIN);
      localStorage.removeItem(KEY_USER_LOGIN);
    },
  },
  getters: {
    isAuthenticated: (state) => !!state.userData.token,
    getToken: (state) => state.userData.token ?? "",
  },
});
