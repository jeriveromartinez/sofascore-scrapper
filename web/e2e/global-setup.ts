import { request as playwrightRequest } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { api, AuthRequest, AuthResponse, E2E_ADMIN_TOKEN_PATH, E2E_SEEDED_ADMIN_EMAIL, E2E_SEEDED_ADMIN_PASSWORD } from "./helpers";

const AUTH_PATH = "/api/web/v1/users";

export default async function globalSetup(): Promise<void> {
  const request = await playwrightRequest.newContext({
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:8080",
  });

  try {
    // The seeded administrator is the only account with admin rights on a
    // fresh stack; accounts registered with an invitation are regular users.
    const login = await api.post<AuthResponse, AuthRequest>(
      request,
      `${AUTH_PATH}/login`,
      { email: E2E_SEEDED_ADMIN_EMAIL, password: E2E_SEEDED_ADMIN_PASSWORD, invitationToken: "" },
      AuthRequest,
      AuthResponse,
    );
    if (login.status !== 200 || !login.data?.token) {
      throw new Error(
        `Could not log in as ${E2E_SEEDED_ADMIN_EMAIL}: ${login.error ?? login.status}. ` +
          "The stack seeds that account on first boot; set E2E_SEEDED_ADMIN_EMAIL and " +
          "E2E_SEEDED_ADMIN_PASSWORD for a different target.",
      );
    }
    writeFileSync(E2E_ADMIN_TOKEN_PATH, login.data.token, { mode: 0o600 });
  } finally {
    await request.dispose();
  }
}
