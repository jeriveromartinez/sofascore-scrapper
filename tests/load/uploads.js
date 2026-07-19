import http from "k6/http";
import { check, sleep } from "k6";
import {
  BASE_URL,
  makeAuthHeaders,
  loginUser,
  checkStatus200,
  checkStatus201,
  HEADERS_PROTOBUF,
  encodeUploadBeginRequest,
} from "./common.js";

export const options = {
  duration: "10m",
  vus: 3,
  thresholds: {
    http_req_failed: ["rate<0.05"],
    "http_req_duration{kind:write}": ["p(95)<5000"],
  },
};

const CHUNK_BYTES = new Uint8Array(64 * 1024);
for (let i = 0; i < CHUNK_BYTES.length; i++) {
  CHUNK_BYTES[i] = (i * 7 + 13) & 0xff;
}

export function setup() {
  const token = loginUser(http, "admin@example.com", "admin123");
  return { adminToken: token };
}

export default function (data) {
  const authHeaders = makeAuthHeaders(data.adminToken);
  const checks = {};
  const runId = `k6-${Date.now()}-${__VU}-${__ITER}`;

  const beginBody = encodeUploadBeginRequest(
    `test-${runId}.apk`,
    128 * 1024,
    2,
    "0.0.1",
    `K6 upload test ${runId}`
  );

  const beginRes = http.post(`${BASE_URL}/api/web/v1/apk/uploads`, beginBody, {
    headers: authHeaders,
    tags: { kind: "write" },
  });
  checks["upload begin"] = beginRes.status === 201 || beginRes.status === 409;
  if (beginRes.status !== 201) {
    check(null, checks);
    return;
  }

  let uploadId;
  try {
    uploadId = beginRes.json("upload_id");
  } catch (_) {
    check(null, checks);
    return;
  }

  for (let ci = 0; ci < 2; ci++) {
    const chunkRes = http.put(
      `${BASE_URL}/api/web/v1/apk/uploads/${uploadId}/chunks/${ci}`,
      CHUNK_BYTES.buffer,
      { headers: authHeaders, tags: { kind: "write" } }
    );
    checks[`chunk ${ci}`] = checkStatus200(chunkRes);
  }

  sleep(0.1);

  const completeRes = http.post(
    `${BASE_URL}/api/web/v1/apk/uploads/${uploadId}/complete`,
    null,
    { headers: authHeaders, tags: { kind: "write" } }
  );
  checks["upload complete"] = completeRes.status === 200 || completeRes.status === 409;

  check(null, checks);
}

export function teardown() {}
