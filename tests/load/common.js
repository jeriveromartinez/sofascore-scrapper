export const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";

export const thresholds = {
  http_req_failed: ["rate<0.01"],
  "http_req_duration{kind:read}": ["p(95)<250"],
  "http_req_duration{kind:write}": ["p(95)<500"],
};

export const HEADERS_PROTOBUF = {
  "Content-Type": "application/x-protobuf",
  Accept: "application/x-protobuf",
};

export const HEADERS_JSON = {
  "Content-Type": "application/json",
  Accept: "application/json",
};

export function checkStatus200(res, prefix) {
  const pass = res.status === 200;
  if (!pass) {
    console.error(`${prefix || ""}expected 200, got ${res.status}: ${res.body}`);
  }
  return pass;
}

export function checkStatus201(res, prefix) {
  const pass = res.status === 201;
  if (!pass) {
    console.error(`${prefix || ""}expected 201, got ${res.status}: ${res.body}`);
  }
  return pass;
}

export function checkStatus2xx(res) {
  return res.status >= 200 && res.status < 300;
}

export function checkStatus503(res) {
  return res.status === 503;
}

export function makeAuthHeaders(token) {
  return Object.assign({}, HEADERS_PROTOBUF, {
    Authorization: `Bearer ${token}`,
  });
}

export function makeDeviceHeaders(deviceToken) {
  return Object.assign({}, HEADERS_PROTOBUF, {
    "APP-XIPTV": deviceToken,
  });
}

function encodeVarint(value) {
  const bytes = [];
  do {
    let b = value & 0x7f;
    value >>>= 7;
    if (value !== 0) b |= 0x80;
    bytes.push(b);
  } while (value !== 0);
  return bytes;
}

function encodeTag(fieldNum, wireType) {
  return encodeVarint((fieldNum << 3) | wireType);
}

function encodeBytes(bytes) {
  const len = encodeVarint(bytes.length);
  return len.concat(bytes);
}

function concatByteArrays(...arrays) {
  const totalLen = arrays.reduce((s, a) => s + a.length, 0);
  const result = new Uint8Array(totalLen);
  let offset = 0;
  for (const a of arrays) {
    result.set(a, offset);
    offset += a.length;
  }
  return result;
}

function strBytes(s) {
  const encoder = new TextEncoder();
  return Array.from(encoder.encode(s));
}

function protoStringField(fieldNum, value) {
  const tag = encodeTag(fieldNum, 2);
  const val = strBytes(value);
  const len = encodeVarint(val.length);
  return tag.concat(len).concat(val);
}

function protoVarintField(fieldNum, value) {
  const tag = encodeTag(fieldNum, 0);
  return tag.concat(encodeVarint(value));
}

function toArrayBuffer(bytes) {
  return new Uint8Array(bytes).buffer;
}

export function encodeLoginRequest(email, password) {
  return toArrayBuffer(
    concatByteArrays(
      protoStringField(1, email),
      protoStringField(2, password)
    )
  );
}

export function encodeRegisterRequest(token, platform, name, version) {
  return toArrayBuffer(
    concatByteArrays(
      protoStringField(1, token),
      protoStringField(2, platform),
      protoStringField(3, name),
      protoStringField(4, version)
    )
  );
}

export function encodePlaybackRequest(deviceToken, content, startedAt) {
  return toArrayBuffer(
    concatByteArrays(
      protoStringField(1, deviceToken),
      protoStringField(2, content),
      protoVarintField(3, startedAt)
    )
  );
}

export function encodeUploadBeginRequest(fileName, fileSize, totalChunks, version, description) {
  return toArrayBuffer(
    concatByteArrays(
      protoStringField(1, fileName),
      protoVarintField(2, fileSize),
      protoVarintField(3, totalChunks),
      protoStringField(4, version),
      protoStringField(5, description)
    )
  );
}

export function loginUser(http, email, password) {
  const body = encodeLoginRequest(email, password);
  const res = http.post(`${BASE_URL}/api/web/v1/users/login`, body, {
    headers: HEADERS_PROTOBUF,
  });
  if (res.status !== 200) {
    throw new Error(`login failed: ${res.status} ${res.body}`);
  }
  return res.json("token");
}

export function registerDevice(http, token, platform, name, version) {
  const body = encodeRegisterRequest(token, platform, name, version);
  return http.post(`${BASE_URL}/api/app/v1/devices`, body, {
    headers: HEADERS_PROTOBUF,
  });
}
