import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  checkStatus200,
  makeAuthHeaders,
  makeDeviceHeaders,
  loginUser,
  registerDevice,
} from "./common.js";

export const options = {
  duration: "5m",
  vus: 15,
  thresholds: {
    http_req_failed: ["rate<0.02"],
    "http_req_duration{kind:read}": ["p(95)<300"],
    "http_req_duration{kind:write}": ["p(95)<600"],
  },
};

const DEVICE_BASE = "k6-multi-inst-";

export function setup() {
  const token = loginUser(http, "admin@example.com", "admin123");

  const devices = [];
  for (let i = 0; i < 10; i++) {
    const devToken = DEVICE_BASE + i;
    const res = registerDevice(http, devToken, "android", `MultiInst ${i}`, "1.0.0");
    if (res.status === 200 || res.status === 409) {
      devices.push({ token: devToken });
    }
  }
  return { adminToken: token, devices };
}

export default function (data) {
  const checks = {};
  const adminHeaders = makeAuthHeaders(data.adminToken);
  const device = data.devices[__VU % data.devices.length];
  const devHeaders = makeDeviceHeaders(device.token);

  const health = http.get(`${BASE_URL}/health/live`);
  checks["health/live"] = checkStatus200(health);

  const ready = http.get(`${BASE_URL}/health/ready`);
  checks["health/ready"] = ready.status === 200 || ready.status === 503;

  const events = http.get(`${BASE_URL}/api/app/v1/current-events?limit=4`, {
    headers: devHeaders,
    tags: { kind: "read" },
  });
  checks["current-events"] = checkStatus200(events);

  const adminEvents = http.get(
    `${BASE_URL}/api/web/v1/events/page?limit=10`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["admin events page"] = checkStatus200(adminEvents);

  const devicesList = http.get(
    `${BASE_URL}/api/web/v1/devices?page=1&limit=5`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["admin devices"] = checkStatus200(devicesList);

  const stats = http.get(
    `${BASE_URL}/api/web/v1/stats/top-events?limit=5`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["stats"] = checkStatus200(stats);

  const metrics = http.get(`${BASE_URL}/metrics`);
  checks["metrics"] = checkStatus200(metrics);

  check(null, checks);
}

export function teardown() {}
