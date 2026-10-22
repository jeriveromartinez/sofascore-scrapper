// Wire-format tests for the protobuf helpers used by the k6 load scripts.
//
// The encoders/decoders here are written from proto/api.proto without
// reusing common.js, so a mistake in one side cannot mask a mistake in
// the other.
//
// Run locally with:  node --test tests/load/common.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";

// common.js reads the k6-only global __ENV at module scope and refuses to
// load without an admin password.
globalThis.__ENV = { ADMIN_PASSWORD: "test-admin-password" };

const {
  decodeProtoStringField,
  decodeProtoVarintField,
  encodeAuthRequest,
  encodeCreateInvitationRequest,
  encodePlaybackRequest,
  encodeUploadBeginRequest,
  AUTH_ID_FIELD,
  AUTH_TOKEN_FIELD,
  INVITE_TOKEN_FIELD,
  UPLOAD_ID_FIELD,
} = await import("./common.js");

function refVarint(value) {
  const out = [];
  let remaining = value;
  do {
    let b = remaining % 128;
    remaining = Math.floor(remaining / 128);
    if (remaining !== 0) b += 128;
    out.push(b);
  } while (remaining !== 0);
  return out;
}

function refTag(fieldNum, wireType) {
  return refVarint(fieldNum * 8 + wireType);
}

function refStringField(fieldNum, value) {
  const payload = Array.from(Buffer.from(value, "utf8"));
  return [...refTag(fieldNum, 2), ...refVarint(payload.length), ...payload];
}

function refVarintField(fieldNum, value) {
  return [...refTag(fieldNum, 0), ...refVarint(value)];
}

// Reference reader: returns [fieldNum, value] pairs of a protobuf message.
function refReadFields(bytes) {
  const fields = [];
  let i = 0;
  while (i < bytes.length) {
    let tag = 0;
    let shift = 0;
    while (i < bytes.length) {
      const b = bytes[i++];
      tag += (b & 0x7f) * Math.pow(2, shift);
      if ((b & 0x80) === 0) break;
      shift += 7;
    }
    const fieldNum = Math.floor(tag / 8);
    const wireType = tag % 8;
    if (wireType === 0) {
      let value = 0;
      shift = 0;
      while (i < bytes.length) {
        const b = bytes[i++];
        value += (b & 0x7f) * Math.pow(2, shift);
        if ((b & 0x80) === 0) break;
        shift += 7;
      }
      fields.push([fieldNum, value]);
    } else if (wireType === 2) {
      let len = 0;
      shift = 0;
      while (i < bytes.length) {
        const b = bytes[i++];
        len += (b & 0x7f) * Math.pow(2, shift);
        if ((b & 0x80) === 0) break;
        shift += 7;
      }
      fields.push([fieldNum, Buffer.from(bytes.slice(i, i + len)).toString("utf8")]);
      i += len;
    } else {
      throw new Error(`unexpected wire type ${wireType}`);
    }
  }
  return fields;
}

function refFieldValue(fields, fieldNum) {
  const found = fields.find(([num]) => num === fieldNum);
  return found ? found[1] : undefined;
}

test("AuthResponse is decoded from its field numbers", () => {
  const payload = Buffer.from([
    ...refVarintField(1, 42),
    ...refStringField(2, "admin@example.com"),
    ...refStringField(3, "access-token-value"),
    ...refStringField(4, "refresh-token-value"),
  ]);

  assert.equal(decodeProtoVarintField(payload, AUTH_ID_FIELD), 42);
  assert.equal(decodeProtoStringField(payload, 2), "admin@example.com");
  assert.equal(decodeProtoStringField(payload, AUTH_TOKEN_FIELD), "access-token-value");
  assert.equal(decodeProtoStringField(payload, 4), "refresh-token-value");
});

test("InvitationResponse token is field 1", () => {
  const payload = Buffer.from([
    ...refStringField(1, "invitation-token"),
    ...refVarintField(2, 1758000000),
  ]);

  assert.equal(decodeProtoStringField(payload, INVITE_TOKEN_FIELD), "invitation-token");
  assert.equal(decodeProtoVarintField(payload, 2), 1758000000);
});

test("missing fields decode to empty values", () => {
  const payload = Buffer.from([...refStringField(3, "only-token")]);

  assert.equal(decodeProtoStringField(payload, 1), "");
  assert.equal(decodeProtoVarintField(payload, 1), 0);
});

test("string fields are decoded as UTF-8", () => {
  const payload = Buffer.from([...refStringField(2, "üser@exämple.com")]);

  assert.equal(decodeProtoStringField(payload, 2), "üser@exämple.com");
});

test("AuthRequest encodes email, password and invitation token", () => {
  const payload = Buffer.from(
    encodeAuthRequest("admin@example.com", "Password1!", "invitation-token"),
  );
  const fields = refReadFields(payload);

  assert.equal(refFieldValue(fields, 1), "admin@example.com");
  assert.equal(refFieldValue(fields, 2), "Password1!");
  assert.equal(refFieldValue(fields, 3), "invitation-token");
});

test("CreateInvitationRequest encodes ttl_seconds as a varint", () => {
  const payload = Buffer.from(encodeCreateInvitationRequest(600));
  const fields = refReadFields(payload);

  assert.equal(refFieldValue(fields, 1), 600);
});

test("int64 fields above 2^31 keep their value", () => {
  const startedAt = 1758000000000;
  const payload = Buffer.from(encodePlaybackRequest("device-token", "channel", startedAt));
  const fields = refReadFields(payload);

  assert.equal(refFieldValue(fields, 3), startedAt);
});

test("file sizes above 2^31 keep their value", () => {
  const fileSize = 5 * 1024 * 1024 * 1024;
  const payload = Buffer.from(encodeUploadBeginRequest("app.apk", fileSize, 2, "1.0.0", "desc"));
  const fields = refReadFields(payload);

  assert.equal(refFieldValue(fields, 2), fileSize);
  assert.equal(refFieldValue(fields, 3), 2);
  assert.equal(refFieldValue(fields, 4), "1.0.0");
  assert.equal(refFieldValue(fields, 5), "desc");
});

test("UploadBeginResponse upload_id is field 1", () => {
  const payload = Buffer.from([...refStringField(1, "upload-id-1234")]);

  assert.equal(decodeProtoStringField(payload, UPLOAD_ID_FIELD), "upload-id-1234");
});

// Each case imports a fresh copy of common.js, because __ENV is read
// when the module is evaluated.
async function importCommonWith(env, label) {
  const previous = globalThis.__ENV;
  globalThis.__ENV = env;
  try {
    return await import(`./common.js?case=${label}`);
  } finally {
    globalThis.__ENV = previous;
  }
}

test("loading without ADMIN_PASSWORD fails fast instead of using a default", async () => {
  await assert.rejects(importCommonWith({}, "missing"), /ADMIN_PASSWORD/);
  await assert.rejects(importCommonWith({ ADMIN_PASSWORD: "" }, "empty"), /ADMIN_PASSWORD/);
});

test("ADMIN_PASSWORD and ADMIN_EMAIL come from the environment", async () => {
  const mod = await importCommonWith(
    { ADMIN_EMAIL: "ops@example.com", ADMIN_PASSWORD: "from-env" },
    "set",
  );

  assert.equal(mod.ADMIN_EMAIL, "ops@example.com");
  assert.equal(mod.ADMIN_PASSWORD, "from-env");
});
