import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  thresholds,
  HEADERS_PROTOBUF,
  makeAuthHeaders,
  encodeLoginRequest,
  encodeAuthRequest,
  encodeCreateInvitationRequest,
  encodeSetUserRoleRequest,
  decodeProtoStringField,
  decodeProtoVarintField,
} from "./common.js";

export { thresholds };

// Admin endpoints are rate-limited to 300/min PER USER, so a single operator
// cannot sustain real load. We seed a pool of admin accounts and drive a
// controlled arrival rate that stays under the aggregate budget
// (POOL_SIZE * 300/min), which lets us measure endpoint latency under
// sustained concurrency instead of just probing that they respond.
const PASSWORD = "Password1!";
const INVITE_TOKEN = __ENV.INVITE_TOKEN || "";
// Bounded by the auth rate limit (10 registrations/min per IP) consumed while
// seeding: one bootstrap admin + (POOL_SIZE - 1) invited-then-promoted admins.
const POOL_SIZE = 6;

// AuthResponse: id=1, token=3. InvitationResponse: token=1.
const AUTH_ID_FIELD = 1;
const AUTH_TOKEN_FIELD = 3;
const INVITE_TOKEN_FIELD = 1;

export const options = {
  scenarios: {
    admin_reads: {
      executor: "ramping-arrival-rate",
      startRate: 5,
      timeUnit: "1s",
      preAllocatedVUs: 20,
      maxVUs: 60,
      stages: [
        { target: 12, duration: "30s" }, // ramp up
        { target: 12, duration: "60s" }, // sustained load
        { target: 0, duration: "15s" }, // ramp down
      ],
    },
  },
  thresholds,
};

const ENDPOINTS = [
  "/api/web/v1/devices?page=1&limit=5",
  "/api/web/v1/events?page=1&limit=5",
  "/api/web/v1/apk/versions",
  "/api/web/v1/stats/top-events?limit=5",
];

function bootstrapAdminToken() {
  const params = { headers: HEADERS_PROTOBUF, responseType: "binary" };

  // Fresh stack: register the first account, which is promoted to admin
  // server-side. Fall back to login if it already exists.
  if (INVITE_TOKEN) {
    const reg = http.post(
      `${BASE_URL}/api/web/v1/users/register`,
      encodeAuthRequest("admin@example.com", PASSWORD, INVITE_TOKEN),
      params,
    );
    if (reg.status === 201) {
      return decodeProtoStringField(reg.body, AUTH_TOKEN_FIELD);
    }
  }

  const login = http.post(
    `${BASE_URL}/api/web/v1/users/login`,
    encodeLoginRequest("admin@example.com", PASSWORD),
    params,
  );
  if (login.status === 200) {
    return decodeProtoStringField(login.body, AUTH_TOKEN_FIELD);
  }
  throw new Error(`could not bootstrap admin (register/login failed): ${login.status}`);
}

export function setup() {
  const bootstrapToken = bootstrapAdminToken();
  const adminHeaders = makeAuthHeaders(bootstrapToken);
  const tokens = [bootstrapToken];

  for (let i = 1; i < POOL_SIZE; i++) {
    const inv = http.post(
      `${BASE_URL}/api/web/v1/users/invitations`,
      encodeCreateInvitationRequest(600),
      { headers: adminHeaders, responseType: "binary" },
    );
    if (inv.status !== 201) continue;
    const inviteToken = decodeProtoStringField(inv.body, INVITE_TOKEN_FIELD);

    const reg = http.post(
      `${BASE_URL}/api/web/v1/users/register`,
      encodeAuthRequest(`k6-admin-${i}@load.local`, PASSWORD, inviteToken),
      { headers: HEADERS_PROTOBUF, responseType: "binary" },
    );
    if (reg.status !== 201) continue;
    const id = decodeProtoVarintField(reg.body, AUTH_ID_FIELD);
    const token = decodeProtoStringField(reg.body, AUTH_TOKEN_FIELD);

    // Promote to admin so this token can serve admin reads. RequireAdmin reads
    // the role from the DB, so the pre-promotion token stays valid.
    const promote = http.put(
      `${BASE_URL}/api/web/v1/users/${id}/role`,
      encodeSetUserRoleRequest("admin"),
      { headers: adminHeaders },
    );
    if (promote.status === 200) {
      tokens.push(token);
    }
  }

  if (tokens.length === 0) {
    throw new Error("failed to seed any admin tokens");
  }
  return { tokens };
}

export default function (data) {
  const token = data.tokens[Math.floor(Math.random() * data.tokens.length)];
  const path = ENDPOINTS[Math.floor(Math.random() * ENDPOINTS.length)];
  const res = http.get(`${BASE_URL}${path}`, {
    headers: makeAuthHeaders(token),
    tags: { kind: "read" },
  });
  check(res, { "admin read is 200": (r) => r.status === 200 });
}

export function teardown() {}
