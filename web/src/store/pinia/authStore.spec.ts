import { describe, it, expect, beforeEach } from "vitest";
import { setActivePinia, createPinia } from "pinia";
import { useAuthStore } from "./authStore";
import { clearAuthStorage } from "../authStorage";
import { KEY_USER_LOGIN } from "../../constants";

const fixture = { id: 1, email: "a@b.com", token: "t", refreshToken: "rt" };

describe("authStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    clearAuthStorage();
  });

  it("initializes with empty userData when no storage", () => {
    const store = useAuthStore();
    expect(store.userData.token).toBeUndefined();
  });

  it("reads userData from sessionStorage on init", () => {
    sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
    setActivePinia(createPinia());
    const store = useAuthStore();
    expect(store.userData.token).toBe("t");
  });

  it("setUser writes to sessionStorage without rememberMe", () => {
    const store = useAuthStore();
    store.setUser(fixture, false);
    expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBe(
      JSON.stringify(fixture),
    );
  });

  it("setUser writes to localStorage with rememberMe", () => {
    const store = useAuthStore();
    store.setUser(fixture, true);
    expect(localStorage.getItem(KEY_USER_LOGIN)).toBe(
      JSON.stringify(fixture),
    );
  });

  it("clearUser removes from both storages", () => {
    sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
    localStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
    const store = useAuthStore();
    store.clearUser();
    expect(store.userData.token).toBeUndefined();
    expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBeNull();
    expect(localStorage.getItem(KEY_USER_LOGIN)).toBeNull();
  });

  it("isAuthenticated returns true when token present", () => {
    sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
    setActivePinia(createPinia());
    const store = useAuthStore();
    expect(store.isAuthenticated).toBe(true);
  });

  it("isAuthenticated returns false when no token", () => {
    const store = useAuthStore();
    expect(store.isAuthenticated).toBe(false);
  });

  it("clearUser resets userData to empty object", () => {
    sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
    setActivePinia(createPinia());
    const store = useAuthStore();
    store.clearUser();
    expect(store.userData.token).toBeUndefined();
    expect(store.isAuthenticated).toBe(false);
  });
});
