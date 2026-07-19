import http from "k6/http";
import { check, sleep } from "k6";
import {
  BASE_URL,
  thresholds,
  HEADERS_PROTOBUF,
  checkStatus200,
  makeAuthHeaders,
  encodeLoginRequest,
  encodeAuthRequest,
  decodeProtoStringField,
} from "./common.js";

// AuthResponse.token is protobuf field 3.
const AUTH_TOKEN_FIELD = 3;

export { thresholds };

const SMOKE_THRESHOLDS = Object.assign({}, thresholds, {
  http_req_duration: ["p(95)<800"],
});
export const options = {
  duration: "2m",
  vus: 5,
  thresholds: SMOKE_THRESHOLDS,
};

const LOGIN_EMAIL = __ENV.TEST_EMAIL || "admin@example.com";
const LOGIN_PASSWORD = __ENV.TEST_PASSWORD || "admin123";
const INVITE_TOKEN = __ENV.INVITE_TOKEN || "";

export function setup() {
  // Responses are protobuf; request binary bodies and decode the token field.
  const params = { headers: HEADERS_PROTOBUF, responseType: "binary" };
  let res = http.post(
    `${BASE_URL}/api/web/v1/users/login`,
    encodeLoginRequest(LOGIN_EMAIL, LOGIN_PASSWORD),
    params,
  );

  // On a fresh stack the operator does not exist yet; register it with the
  // bootstrap invitation. The first account is promoted to admin server-side,
  // which is required for the admin endpoints exercised below.
  if (res.status !== 200 && INVITE_TOKEN) {
    res = http.post(
      `${BASE_URL}/api/web/v1/users/register`,
      encodeAuthRequest(LOGIN_EMAIL, LOGIN_PASSWORD, INVITE_TOKEN),
      params,
    );
    if (res.status !== 201) {
      throw new Error(`setup register failed: ${res.status}`);
    }
  } else if (res.status !== 200) {
    throw new Error(`setup login failed: ${res.status}`);
  }

  const token = decodeProtoStringField(res.body, AUTH_TOKEN_FIELD);
  if (!token) {
    throw new Error("setup could not extract token from auth response");
  }
  return { token };
}

export default function (data) {
  const authHeaders = makeAuthHeaders(data.token);
  const checks = {};

  const health = http.get(`${BASE_URL}/health/live`);
  checks["health/live"] = checkStatus200(health);

  const ready = http.get(`${BASE_URL}/health/ready`);
  checks["health/ready"] = checkStatus200(ready);

  const devices = http.get(`${BASE_URL}/api/web/v1/devices?page=1&limit=5`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["admin devices"] = checkStatus200(devices);

  const events = http.get(`${BASE_URL}/api/web/v1/events?page=1&limit=5`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["admin events"] = checkStatus200(events);

  const versions = http.get(`${BASE_URL}/api/web/v1/apk/versions`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["apk versions"] = checkStatus200(versions);

  const top = http.get(`${BASE_URL}/api/web/v1/stats/top-events`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["stats top-events"] = checkStatus200(top);

  check(null, checks);

  // Think-time keeps the shared admin bucket (RateLimitAdmin = 300/min, keyed
  // per user) from tripping: 5 VUs x 4 admin calls / 5s ~= 240 req/min.
  sleep(5);
}

export function teardown() {}
