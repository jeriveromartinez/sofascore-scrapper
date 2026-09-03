import { describe, it, expect, vi } from "vitest";
import {
  FeatureFlagApiService,
  PushesApiService,
  featureFlagApiService,
  pushesApiService,
} from "./PushesApiService";
import type {
  CreateImmediatePushPayload,
  CreateSchedulePayload,
  PushPayload,
  SetNotificationsEnabledPayload,
  UpdateSchedulePayload,
} from "./models";

/**
 * The push service encodes/decodes via ts-proto MessageFns.
 * The spec does not care about the wire format (covered by
 * generated code); it only verifies that each method routes to
 * the right HTTP verb + URL + body, and that the typed payload
 * flows through untouched. We stub the BaseApiService helpers
 * (post/patch/put/get/delete) with vi.fn()s and inspect their
 * arguments.
 */
type Stub = {
  get: ReturnType<typeof vi.fn>;
  post: ReturnType<typeof vi.fn>;
  put: ReturnType<typeof vi.fn>;
  patch: ReturnType<typeof vi.fn>;
  del: ReturnType<typeof vi.fn>;
};

function makeService(): { svc: PushesApiService } & Stub {
  const svc = new PushesApiService();
  const get = vi.fn(async () => ({}));
  const post = vi.fn(async () => ({}));
  const put = vi.fn(async () => ({}));
  const patch = vi.fn(async () => ({}));
  const del = vi.fn(async () => ({}));
  (svc as unknown as { get: typeof get }).get = get;
  (svc as unknown as { post: typeof post }).post = post;
  (svc as unknown as { put: typeof put }).put = put;
  (svc as unknown as { patch: typeof patch }).patch = patch;
  (svc as unknown as { delete: typeof del }).delete = del;
  return { svc, get, post, put, patch, del };
}

function makeFlagService(): { svc: FeatureFlagApiService; put: ReturnType<typeof vi.fn> } {
  const svc = new FeatureFlagApiService();
  const put = vi.fn(async () => ({}));
  (svc as unknown as { put: typeof put }).put = put;
  return { svc, put };
}

// Minimal payload that satisfies the proto3 "every field is
// optional on the wire" semantics. The TS interface declares
// every field as required for type safety, but the wire format
// is fine with just the fields we actually want to send.
function payload(partial: Partial<PushPayload> = {}): PushPayload {
  return {
    category: 0,
    priority: 0,
    title: "",
    body: "",
    imageUrl: "",
    ttlSeconds: 0,
    data: {},
    ...partial,
  };
}

describe("PushesApiService.createImmediatePush", () => {
  it("POSTs to /pushes with the typed payload and proto codec", async () => {
    const { svc, post } = makeService();
    const body: CreateImmediatePushPayload = {
      domainIds: [1, 2],
      payload: payload({ title: "hi", body: "world" }),
    };
    await svc.createImmediatePush(body);
    expect(post).toHaveBeenCalledTimes(1);
    const call = post.mock.calls[0];
    expect(call?.[0]).toBe("");
    expect(call?.[1]).toBe(body);
  });
});

describe("PushesApiService.getPush", () => {
  it("GETs /pushes/:id", async () => {
    const { svc, get } = makeService();
    await svc.getPush(42);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("/42");
  });
});

describe("PushesApiService.listPushes", () => {
  it("returns the bare URL when no cursor or limit are provided", async () => {
    const { svc, get } = makeService();
    await svc.listPushes();
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("");
  });

  it("serializes cursor and limit into the query string", async () => {
    const { svc, get } = makeService();
    await svc.listPushes("cur-123", 25);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("?cursor=cur-123&limit=25");
  });

  it("omits cursor when only limit is provided", async () => {
    const { svc, get } = makeService();
    await svc.listPushes(undefined, 10);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("?limit=10");
  });
});

describe("PushesApiService schedules", () => {
  it("createSchedule POSTs to /pushes/schedules", async () => {
    const { svc, post } = makeService();
    const body: CreateSchedulePayload = {
      domainIds: [3],
      payload: payload({ title: "t", body: "b" }),
      scheduleType: 1,
      runAt: "2026-12-01T10:00:00Z",
      cronExpr: "",
    };
    await svc.createSchedule(body);
    const call = post.mock.calls[0];
    expect(call?.[0]).toBe("/schedules");
    expect(call?.[1]).toBe(body);
  });

  it("listSchedules serializes cursor and limit", async () => {
    const { svc, get } = makeService();
    await svc.listSchedules("c", 50);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("/schedules?cursor=c&limit=50");
  });

  it("getSchedule hits /pushes/schedules/:id", async () => {
    const { svc, get } = makeService();
    await svc.getSchedule(7);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("/schedules/7");
  });

  it("updateSchedule PATCHes /pushes/schedules/:id", async () => {
    const { svc, patch } = makeService();
    const body: UpdateSchedulePayload = {
      id: 7,
      isActive: false,
      payload: payload(),
    };
    await svc.updateSchedule(7, body);
    const call = patch.mock.calls[0];
    expect(call?.[0]).toBe("/schedules/7");
    expect(call?.[1]).toBe(body);
  });

  it("deleteSchedule DELETEs /pushes/schedules/:id", async () => {
    const { svc, del } = makeService();
    await svc.deleteSchedule(7);
    const call = del.mock.calls[0];
    expect(call?.[0]).toBe("/schedules/7");
  });
});

describe("PushesApiService metrics", () => {
  it("getAggregateMetrics GETs /pushes/metrics/aggregate", async () => {
    const { svc, get } = makeService();
    await svc.getAggregateMetrics();
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("/metrics/aggregate");
  });

  it("getCampaignMetrics GETs /pushes/metrics/campaign/:id", async () => {
    const { svc, get } = makeService();
    await svc.getCampaignMetrics(99);
    const call = get.mock.calls[0];
    expect(call?.[0]).toBe("/metrics/campaign/99");
  });
});

describe("FeatureFlagApiService.setNotificationsEnabled", () => {
  it("PUTs /users/:id/notifications with the typed payload", async () => {
    const { svc, put } = makeFlagService();
    const body: SetNotificationsEnabledPayload = { enabled: true };
    await svc.setNotificationsEnabled(5, body);
    const call = put.mock.calls[0];
    expect(call?.[0]).toBe("/5/notifications");
    expect(call?.[1]).toBe(body);
  });

  it("toggles off when payload.enabled = false", async () => {
    const { svc, put } = makeFlagService();
    const body: SetNotificationsEnabledPayload = { enabled: false };
    await svc.setNotificationsEnabled(11, body);
    // The second arg to put() is the typed payload (third is the codec).
    const call = put.mock.calls[0];
    expect(call?.[1]).toBe(body);
  });
});

describe("PushesApiService singleton instances", () => {
  it("pushesApiService is a PushesApiService", () => {
    expect(pushesApiService).toBeInstanceOf(PushesApiService);
  });

  it("featureFlagApiService is a FeatureFlagApiService", () => {
    expect(featureFlagApiService).toBeInstanceOf(FeatureFlagApiService);
  });
});
