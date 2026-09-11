import axios, {
  type AxiosInstance,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from "axios";
import { API_BASE_URL } from "../../constants";
import { clearAuthStorage } from "../authStorage";
import { refreshAuth, ApiError } from "./BaseApiService";

/**
 * Routes the user back to the login screen when the refresh token is no
 * longer accepted. Mirrors the helper inside BaseApiService — kept local
 * because that helper isn't exported and we want to avoid widening the
 * BaseApiService surface for a single consumer.
 */
function redirectToLogin(): void {
  import("../../router").then(({ router }) => {
    router.push({ name: "Login" });
  });
}

const SKIP_REFRESH_PATHS = [
  "/users/login",
  "/users/register",
  "/users/refresh",
];

function isSkippedPath(url: string): boolean {
  return SKIP_REFRESH_PATHS.some((p) => url.includes(p));
}

function applyBearer(
  config: InternalAxiosRequestConfig,
  token: string,
): void {
  // Axios may present headers as either a plain object or AxiosHeaders —
  // support both so this works regardless of how the caller set things up.
  const headers = config.headers as unknown as Record<string, string> & {
    set?: (k: string, v: string) => void;
  };
  if (headers && typeof headers.set === "function") {
    headers.set("Authorization", `Bearer ${token}`);
  } else {
    (config.headers as Record<string, string>).Authorization =
      `Bearer ${token}`;
  }
}

async function retryWithRefresh(
  config: InternalAxiosRequestConfig & { _retry?: boolean },
  instance: AxiosInstance,
): Promise<AxiosResponse> {
  config._retry = true;
  const nextUser = await refreshAuth();
  if (!nextUser?.token) {
    clearAuthStorage();
    redirectToLogin();
    throw new ApiError("Authentication failed", 401, true);
  }
  applyBearer(config, nextUser.token);
  return instance.request(config);
}

/**
 * Build a JSON-axios instance that re-uses the same 401 refresh-and-retry
 * behavior as `BaseApiService`. Used by services that don't extend
 * `BaseApiService` (e.g. ScraperLeagueService, which speaks plain JSON
 * rather than protobuf).
 *
 * The interceptor catches 401 in BOTH the success path (when callers opt
 * in with `validateStatus: () => true`) and the standard error path
 * (default axios behavior), so the retry actually fires regardless of
 * how the consumer drives the request.
 */
export function createAuthJsonAxios(): AxiosInstance {
  const instance = axios.create({ baseURL: API_BASE_URL });

  instance.interceptors.response.use(
    async (response: AxiosResponse) => {
      const config = response.config as InternalAxiosRequestConfig & {
        _retry?: boolean;
      };
      if (response.status !== 401 || config._retry) return response;
      const url = config.url ?? "";
      if (isSkippedPath(url)) return response;
      return retryWithRefresh(config, instance);
    },
    async (error: unknown) => {
      const err = error as {
        response?: { status?: number };
        config?: InternalAxiosRequestConfig & { _retry?: boolean };
      };
      const status = err?.response?.status;
      const config = err?.config;
      if (status !== 401 || !config || config._retry) {
        throw error;
      }
      const url = config.url ?? "";
      if (isSkippedPath(url)) throw error;
      return retryWithRefresh(config, instance);
    },
  );

  return instance;
}

/**
 * Shared singleton — same shape as the default `axios` export so call
 * sites stay terse (e.g. `authJsonAxios.get(url, opts)`).
 */
export const authJsonAxios: AxiosInstance = createAuthJsonAxios();
