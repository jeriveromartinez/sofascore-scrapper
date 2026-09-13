import { test, expect } from "@playwright/test";
import {
  api,
  AuthRequest,
  AuthResponse,
  createTestInvitation,
  getE2EAdminToken,
} from "./helpers";
import { SetUserRoleRequest, UserList } from "../src/proto/api";

const AUTH_PATH = "/api/web/v1/users";
const USERS_PATH = "/api/web/v1/users";
const USER_INFO_KEY = "user_info";

const TEST_EMAIL = `e2e-role-${Date.now()}@test.local`;
const TEST_PASSWORD = "Password1!";

async function injectAdminSession(page: import("@playwright/test").Page): Promise<void> {
  const token = getE2EAdminToken();
  const user = {
    id: 0,
    email: "e2e-admin@test.local",
    token,
    refreshToken: token,
  };
  await page.addInitScript(
    ([key, value]) => {
      localStorage.setItem(key, value);
    },
    [USER_INFO_KEY, JSON.stringify(user)],
  );
}

test.describe("Admin > User edit role", () => {
  let targetUserId = 0;

  test.beforeAll(async ({ request }) => {
    // Bootstrap a fresh "user"-role account via invitation +
    // registration, the same flow that an operator would use to
    // onboard a teammate.
    const invitationToken = await createTestInvitation(request);
    const reg = await api.post<AuthResponse, AuthRequest>(
      request,
      `${AUTH_PATH}/register`,
      { email: TEST_EMAIL, password: TEST_PASSWORD, invitationToken },
      AuthRequest,
      AuthResponse,
    );
    expect(reg.status, `register error: ${reg.error}`).toBe(201);
    targetUserId = reg.data!.id;
  });

  test.beforeEach(async ({ page }) => {
    await injectAdminSession(page);
  });

  test("edit modal exposes a role <select> pre-filled with the current role", async ({ page }) => {
    await page.goto("/#/users");
    await page.waitForLoadState("networkidle");

    // Find the row for our target user and click its Edit button.
    const row = page.locator("table tbody tr", { hasText: TEST_EMAIL });
    await expect(row).toHaveCount(1);
    await row.locator('button:has-text("Editar")').click();

    // Edit modal opens with the role select pre-filled.
    const modal = page.locator(".modal.show");
    await expect(modal).toBeVisible();
    await expect(modal.locator("h5")).toContainText("Editar usuario");

    const roleSelect = modal.locator('select[data-testid="user-role"]');
    await expect(roleSelect).toBeVisible();
    await expect(roleSelect).toHaveValue("user");
  });

  test("promoting a user to admin persists via PUT /users/:id/role", async ({ page, request }) => {
    await page.goto("/#/users");
    await page.waitForLoadState("networkidle");

    const row = page.locator("table tbody tr", { hasText: TEST_EMAIL });
    await row.locator('button:has-text("Editar")').click();

    // Change the role to admin and submit.
    const modal = page.locator(".modal.show");
    const roleSelect = modal.locator('select[data-testid="user-role"]');
    await roleSelect.selectOption("admin");
    await modal.locator('button:has-text("Actualizar")').click();

    // The modal closes on success; the table should still show the row.
    await expect(page.locator(".modal.show")).toHaveCount(0);

    // Verify server-side via the users list endpoint using the admin
    // token, independent of the Vue UI cache. Pull a single page of
    // users and look up the email — this catches the case where the
    // UI lies but the API actually saved it.
    //
    // The list endpoint speaks application/x-protobuf (UserList),
    // not JSON. We decode with the proto helper instead of calling
    // resp.json(), which would throw on the binary body and cause
    // the assertion to be silently skipped.
    const token = getE2EAdminToken();
    const list = await api.get(request, USERS_PATH, UserList, {
      Authorization: `Bearer ${token}`,
    });
    expect(list.status, `users list error: ${list.error}`).toBe(200);
    const target = list.data?.users.find((u) => u.email === TEST_EMAIL);
    expect(target, `target user ${TEST_EMAIL} not in users list`).toBeTruthy();
    expect(target!.role).toBe("admin");

    // Avoid leaking the elevated role into other tests.
    const resetPayload = SetUserRoleRequest.encode({ role: "user" }).finish();
    await request.put(`${USERS_PATH}/${targetUserId}/role`, {
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/x-protobuf",
      },
      data: Buffer.from(resetPayload),
    });
  });
});
