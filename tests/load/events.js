import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  thresholds,
  HEADERS_PROTOBUF,
  checkStatus200,
  makeAuthHeaders,
  makeDeviceHeaders,
  loginUser,
  registerDevice,
  encodeRegisterRequest,
} from "./common.js";

export { thresholds };

export const options = {
  duration: "30m",
  vus: 50,
  thresholds,
};

const DEVICE_BASE_TOKEN = "k6-load-events-";

export function setup() {
  const token = loginUser(http, "admin@example.com", "admin123");

  const devices = [];
  for (let i = 0; i < 20; i++) {
    const devToken = DEVICE_BASE_TOKEN + i;
    const res = registerDevice(http, devToken, "android", `K6 Events ${i}`, "1.0.0");
    if (res.status === 200 || res.status === 409) {
      try {
        devices.push({ token: devToken });
      } catch (_) {}
    }
  }
  return { adminToken: token, devices };
}

export default function (data) {
  const checks = {};
  const adminHeaders = makeAuthHeaders(data.adminToken);
  const device = data.devices[Math.floor(Math.random() * data.devices.length)];

  const events = http.get(`${BASE_URL}/api/app/v1/current-events?limit=6`, {
    headers: makeDeviceHeaders(device.token),
    tags: { kind: "read" },
  });
  checks["current-events"] = checkStatus200(events);

  const adminEvents = http.get(
    `${BASE_URL}/api/web/v1/events?page=1&limit=10`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["admin events"] = checkStatus200(adminEvents);

  const adminEventsPage = http.get(
    `${BASE_URL}/api/web/v1/events/page?limit=20`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["admin events page"] = checkStatus200(adminEventsPage);

  const topStats = http.get(
    `${BASE_URL}/api/web/v1/stats/top-events?limit=5`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["top-events"] = checkStatus200(topStats);

  check(null, checks);
}

export function teardown() {}
