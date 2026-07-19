import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  thresholds,
  checkStatus200,
  makeAuthHeaders,
  loginUser,
} from "./common.js";

export { thresholds };

export const options = {
  duration: "30m",
  vus: 20,
  thresholds,
};

export function setup() {
  const token = loginUser(http, "admin@example.com", "admin123");
  return { adminToken: token };
}

export default function (data) {
  const checks = {};
  const authHeaders = makeAuthHeaders(data.adminToken);

  const playback = http.get(`${BASE_URL}/api/web/v1/playback?page=1&limit=10`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["playback list"] = checkStatus200(playback);

  const page = http.get(`${BASE_URL}/api/web/v1/playback/page?limit=20`, {
    headers: authHeaders,
    tags: { kind: "read" },
  });
  checks["playback page"] = checkStatus200(page);

  check(null, checks);
}

export function teardown() {}
