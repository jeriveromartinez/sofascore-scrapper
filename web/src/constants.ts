export const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";

export const API_ORIGIN = new URL(API_BASE_URL).origin;
export const KEY_USER_LOGIN = "user_info";
