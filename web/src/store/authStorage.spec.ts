import { describe, it, expect, beforeEach } from "vitest";
import { readAuthStorage, writeAuthStorage, clearAuthStorage } from "./authStorage";
import { KEY_USER_LOGIN } from "../constants";

const fixture = { id: 1, email: "a@b.com", token: "t", refreshToken: "rt" };

describe("authStorage", () => {
  beforeEach(() => {
    clearAuthStorage();
  });

  describe("readAuthStorage", () => {
    it("returns null user when nothing stored", () => {
      const { user, storage } = readAuthStorage();
      expect(user).toBeNull();
      expect(storage).toBeNull();
    });

    it("reads from sessionStorage", () => {
      sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      const { user, storage } = readAuthStorage();
      expect(user).toEqual(fixture);
      expect(storage).toBe(sessionStorage);
    });

    it("reads from localStorage when session is empty", () => {
      localStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      const { user, storage } = readAuthStorage();
      expect(user).toEqual(fixture);
      expect(storage).toBe(localStorage);
    });

    it("prefers sessionStorage over localStorage", () => {
      sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      localStorage.setItem(KEY_USER_LOGIN, JSON.stringify({ ...fixture, id: 99 }));
      const { user, storage } = readAuthStorage();
      expect(user).toEqual(fixture);
      expect(storage).toBe(sessionStorage);
    });

    it("removes corrupt entry and returns null", () => {
      sessionStorage.setItem(KEY_USER_LOGIN, "{bad json");
      const { user, storage } = readAuthStorage();
      expect(user).toBeNull();
      expect(storage).toBeNull();
      expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBeNull();
    });

    it("falls through to localStorage when session is corrupt", () => {
      sessionStorage.setItem(KEY_USER_LOGIN, "{bad");
      localStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      const { user, storage } = readAuthStorage();
      expect(user).toEqual(fixture);
      expect(storage).toBe(localStorage);
      expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBeNull();
    });
  });

  describe("writeAuthStorage", () => {
    it("writes to sessionStorage when rememberMe=false", () => {
      writeAuthStorage(fixture, false);
      expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBe(
        JSON.stringify(fixture),
      );
    });

    it("writes to localStorage when rememberMe=true", () => {
      writeAuthStorage(fixture, true);
      expect(localStorage.getItem(KEY_USER_LOGIN)).toBe(
        JSON.stringify(fixture),
      );
    });
  });

  describe("clearAuthStorage", () => {
    it("clears both storages", () => {
      sessionStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      localStorage.setItem(KEY_USER_LOGIN, JSON.stringify(fixture));
      clearAuthStorage();
      expect(sessionStorage.getItem(KEY_USER_LOGIN)).toBeNull();
      expect(localStorage.getItem(KEY_USER_LOGIN)).toBeNull();
    });
  });
});
