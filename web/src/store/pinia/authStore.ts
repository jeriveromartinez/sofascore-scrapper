import { readAuthStorage, writeAuthStorage, clearAuthStorage } from "../authStorage";
import type { UserAuthModel } from "../services/models";
import { authApiService } from "../services";
import { defineStore } from "pinia";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    userData: (readAuthStorage().user ?? {}) as Partial<UserAuthModel>,
  }),
  actions: {
    setUser(userData: UserAuthModel, rememberMe: boolean) {
      this.userData = userData;
      writeAuthStorage(userData, rememberMe);
    },
    clearUser() {
      this.userData = {};
      clearAuthStorage();
    },
    async logout() {
      try {
        if (this.userData.token) {
          await authApiService.logout();
        }
      } finally {
        this.clearUser();
      }
    },
  },
  getters: {
    isAuthenticated: (state) => !!state.userData.token,
    getToken: (state) => state.userData.token ?? "",
    getRefreshToken: (state) => state.userData.refreshToken ?? "",
  },
});
