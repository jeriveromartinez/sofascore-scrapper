import { request as playwrightRequest } from "@playwright/test";
import { execSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { api, AuthRequest, AuthResponse, E2E_ADMIN_TOKEN_PATH } from "./helpers";

const AUTH_PATH = "/api/web/v1/users";
const ADMIN_EMAIL = "e2e-admin@test.local";
const ADMIN_PASSWORD = "Password1!";

export default async function globalSetup(): Promise<void> {
  const request = await playwrightRequest.newContext({
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:8080",
  });

  try {
    const login = await api.post<AuthResponse, AuthRequest>(
      request,
      `${AUTH_PATH}/login`,
      { email: ADMIN_EMAIL, password: ADMIN_PASSWORD, invitationToken: "" },
      AuthRequest,
      AuthResponse,
    );
    if (login.status === 200 && login.data?.token) {
      writeFileSync(E2E_ADMIN_TOKEN_PATH, login.data.token, { mode: 0o600 });
      return;
    }

    const invitationToken = execSync(
      "docker compose -f ../deployments/docker/compose.test.yml exec -T backend cat /shared/invite.txt",
      { encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
    ).trim();
    const registration = await api.post<AuthResponse, AuthRequest>(
      request,
      `${AUTH_PATH}/register`,
      { email: ADMIN_EMAIL, password: ADMIN_PASSWORD, invitationToken },
      AuthRequest,
      AuthResponse,
    );
    if (registration.status !== 201 || !registration.data?.token) {
      throw new Error(`Could not register E2E admin: ${registration.error ?? registration.status}`);
    }
    writeFileSync(E2E_ADMIN_TOKEN_PATH, registration.data.token, { mode: 0o600 });
  } finally {
    await request.dispose();
  }
}
