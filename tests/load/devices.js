import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  thresholds,
  HEADERS_PROTOBUF,
  checkStatus200,
  makeDeviceHeaders,
  loginUser,
  registerDevice,
  encodeRegisterRequest,
  encodePlaybackRequest,
} from "./common.js";

export { thresholds };

export const options = {
  duration: "30m",
  vus: 40,
  thresholds,
};

const DEVICE_BASE_TOKEN = "k6-load-dev-";

export function setup() {
  loginUser(http, "admin@example.com", "admin123");

  const devices = [];
  for (let i = 0; i < 30; i++) {
    const token = DEVICE_BASE_TOKEN + i;
    const res = registerDevice(http, token, "android", `K6 Device ${i}`, "1.0.0");
    if (res.status === 200 || res.status === 409) {
      devices.push({ token });
    }
  }
  return { devices };
}

export default function (data) {
  const checks = {};
  const device = data.devices[Math.floor(Math.random() * data.devices.length)];
  const devHeaders = makeDeviceHeaders(device.token);

  const playback = http.post(
    `${BASE_URL}/api/app/v1/devices/viewing`,
    encodePlaybackRequest(device.token, "channel-espn", Date.now()),
    { headers: devHeaders, tags: { kind: "write" } }
  );
  checks["playback report"] = playback.status === 200 || playback.status === 201;

  const events = http.get(`${BASE_URL}/api/app/v1/current-events?limit=3`, {
    headers: devHeaders,
    tags: { kind: "read" },
  });
  checks["current-events"] = checkStatus200(events);

  check(null, checks);
}

export function teardown() {}
