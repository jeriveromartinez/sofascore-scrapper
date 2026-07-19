import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  thresholds,
  checkStatus200,
  checkStatus503,
  makeAuthHeaders,
  makeDeviceHeaders,
  loginUser,
  registerDevice,
  encodeRegisterRequest,
} from "./common.js";

export const options = {
  duration: "5m",
  vus: 10,
  thresholds: {
    http_req_failed: ["rate<0.10"],
    "http_req_duration{kind:read}": ["p(95)<250"],
  },
};

const DEVICE_BASE = "k6-redis-deg-";

export function setup() {
  const token = loginUser(http, "admin@example.com", "admin123");

  const devices = [];
  for (let i = 0; i < 10; i++) {
    const devToken = DEVICE_BASE + i;
    const res = registerDevice(http, devToken, "android", `RedisTest ${i}`, "1.0.0");
    if (res.status === 200 || res.status === 409) {
      devices.push({ token: devToken });
    }
  }
  return { adminToken: token, devices };
}

export default function (data) {
  const checks = {};
  const device = data.devices[__VU % data.devices.length];
  const devHeaders = makeDeviceHeaders(device.token);
  const adminHeaders = makeAuthHeaders(data.adminToken);

  const events = http.get(`${BASE_URL}/api/app/v1/current-events?limit=6`, {
    headers: devHeaders,
    tags: { kind: "read" },
  });
  checks["current-events"] = checkStatus200(events);

  const adminEvents = http.get(
    `${BASE_URL}/api/web/v1/events?page=1&limit=5`,
    { headers: adminHeaders, tags: { kind: "read" } }
  );
  checks["admin events"] = checkStatus200(adminEvents);

  const health = http.get(`${BASE_URL}/health/ready`);
  const readyCheck = health.status === 200 || health.status === 503;
  checks["health ready"] = readyCheck;

  const beginBody = encodeRegisterRequest(
    `k6-rd-${__VU}-${__ITER}`,
    "android",
    `RedisTest-${__VU}`,
    "1.0.0"
  );

  const devReg = http.post(`${BASE_URL}/api/app/v1/devices`, beginBody, {
    headers: { "Content-Type": "application/x-protobuf", Accept: "application/x-protobuf" },
    tags: { kind: "write" },
  });
  checks["device register"] = devReg.status === 200 || devReg.status === 409 || devReg.status === 503;

  check(null, checks);
}

export function teardown() {}
