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

// Manual UTF-8 encoding: the k6 (goja) runtime does not provide TextEncoder.
function strBytes(s) {
  const bytes = [];
  for (let i = 0; i < s.length; i++) {
    let code = s.charCodeAt(i);
    if (code < 0x80) {
      bytes.push(code);
    } else if (code < 0x800) {
      bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
    } else if (code >= 0xd800 && code <= 0xdbff) {
      const lo = s.charCodeAt(++i);
      code = 0x10000 + ((code - 0xd800) << 10) + (lo - 0xdc00);
      bytes.push(
        0xf0 | (code >> 18),
        0x80 | ((code >> 12) & 0x3f),
        0x80 | ((code >> 6) & 0x3f),
        0x80 | (code & 0x3f),
      );
    } else {
      bytes.push(
        0xe0 | (code >> 12),
        0x80 | ((code >> 6) & 0x3f),
        0x80 | (code & 0x3f),
      );
    }
  }
  return bytes;
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

// Minimal protobuf reader: extracts a length-delimited (string) field by number
// from a binary response body. Responses are protobuf-only (no JSON mode), so
// res.json() cannot be used; request with responseType: "binary".
export function decodeProtoStringField(buffer, targetField) {
  const bytes = new Uint8Array(buffer);
  let i = 0;
  while (i < bytes.length) {
    let tag = 0;
    let shift = 0;
    while (i < bytes.length) {
      const b = bytes[i++];
      tag |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7;
    }
    const fieldNum = tag >> 3;
    const wireType = tag & 0x7;
    if (wireType === 2) {
      let len = 0;
      shift = 0;
      while (i < bytes.length) {
        const b = bytes[i++];
        len |= (b & 0x7f) << shift;
        if ((b & 0x80) === 0) break;
        shift += 7;
      }
      if (fieldNum === targetField) {
        let s = "";
        for (let j = 0; j < len; j++) {
          s += String.fromCharCode(bytes[i + j]);
        }
        return s;
      }
      i += len;
    } else if (wireType === 0) {
      while (i < bytes.length && (bytes[i] & 0x80) !== 0) i++;
      i++;
    } else if (wireType === 5) {
      i += 4;
    } else if (wireType === 1) {
      i += 8;
    } else {
      break;
    }
  }
  return "";
}

// Reads a varint (int/uint) field by number from a binary protobuf body.
export function decodeProtoVarintField(buffer, targetField) {
  const bytes = new Uint8Array(buffer);
  let i = 0;
  while (i < bytes.length) {
    let tag = 0;
    let shift = 0;
    while (i < bytes.length) {
      const b = bytes[i++];
      tag |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7;
    }
    const fieldNum = tag >> 3;
    const wireType = tag & 0x7;
    if (wireType === 0) {
      let val = 0;
      shift = 0;
      while (i < bytes.length) {
        const b = bytes[i++];
        val += (b & 0x7f) * Math.pow(2, shift);
        if ((b & 0x80) === 0) break;
        shift += 7;
      }
      if (fieldNum === targetField) return val;
    } else if (wireType === 2) {
      let len = 0;
      shift = 0;
      while (i < bytes.length) {
        const b = bytes[i++];
        len |= (b & 0x7f) << shift;
        if ((b & 0x80) === 0) break;
        shift += 7;
      }
      i += len;
    } else if (wireType === 5) {
      i += 4;
    } else if (wireType === 1) {
      i += 8;
    } else {
      break;
    }
  }
  return 0;
}

export function encodeAuthRequest(email, password, invitationToken) {
  return toArrayBuffer(
    concatByteArrays(
      protoStringField(1, email),
      protoStringField(2, password),
      protoStringField(3, invitationToken)
    )
  );
}

export function encodeCreateInvitationRequest(ttlSeconds) {
  return toArrayBuffer(protoVarintField(1, ttlSeconds));
}

export function encodeSetUserRoleRequest(role) {
  return toArrayBuffer(protoStringField(1, role));
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
