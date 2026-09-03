// PushesApiService
//
// REST client for the push-notification admin surface. The push
// subsystem lives under /api/app/v1/pushes and exposes immediate
// sends, scheduled sends, aggregate and per-campaign metrics, and
// a feature-flag toggle on /users/:id/notifications.
//
// Wire format is application/x-protobuf, encoded via the
// generated ts-proto MessageFns wrappers in ../../proto/api.
//
// All endpoints require the caller's JWT (the
// BaseApiService handles the Authorization header from the auth
// store). The handler chain on the backend also enforces
// notifications_enabled on the caller for create endpoints; the
// UI is expected to gate the compose form on the same flag.
//
// See docs/superpowers/specs/2026-08-28-push-notifications-websocket-design.md
// for the contract.

import { BaseApiService, type ProtoCodec } from "./BaseApiService";
import {
  CreateImmediatePushRequest,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  SetNotificationsEnabledRequest,
  PushMessage as ProtoPushMessage,
  PushMessagePage as ProtoPushMessagePage,
  PushMetricsByCampaign as ProtoPushMetricsByCampaign,
  PushMetricsAggregate as ProtoPushMetricsAggregate,
  ScheduledPush as ProtoScheduledPush,
  ScheduledPushPage as ProtoScheduledPushPage,
  StatusMessage,
  User as ProtoUserMessage,
} from "../../proto/api";
import type {
  CreateImmediatePushPayload,
  CreateSchedulePayload,
  PushMessage,
  PushMessagePageResponse,
  PushMetricsAggregate,
  PushMetricsByCampaign,
  ScheduledPush,
  ScheduledPushPageResponse,
  SetNotificationsEnabledPayload,
  UpdateSchedulePayload,
  User,
} from "./models";

export class PushesApiService extends BaseApiService {
  constructor() {
    super("/pushes");
  }

  // --- immediate pushes ------------------------------------------------

  async createImmediatePush(
    payload: CreateImmediatePushPayload,
  ): Promise<PushMessage> {
    return this.post("", payload, CreateImmediatePushPayloadCodec, ProtoPushMessage);
  }

  async getPush(id: number): Promise<PushMessage> {
    return this.get(`/${id}`, ProtoPushMessage);
  }

  async listPushes(cursor?: string, limit?: number): Promise<PushMessagePageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `?${qs}` : "";
    return this.get(url, ProtoPushMessagePage);
  }

  // --- scheduled pushes ------------------------------------------------

  async createSchedule(
    payload: CreateSchedulePayload,
    options?: { tzMode?: "manager" | "device_local"; managerTz?: string },
  ): Promise<ScheduledPush> {
    const params = new URLSearchParams();
    if (options?.tzMode) params.set("tz_mode", options.tzMode);
    if (options?.managerTz) params.set("manager_tz", options.managerTz);
    const qs = params.toString();
    const url = qs ? `/schedules?${qs}` : "/schedules";
    return this.post(url, payload, CreateSchedulePayloadCodec, ProtoScheduledPush);
  }

  async listSchedules(
    cursor?: string,
    limit?: number,
  ): Promise<ScheduledPushPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/schedules?${qs}` : "/schedules";
    return this.get(url, ProtoScheduledPushPage);
  }

  async getSchedule(id: number): Promise<ScheduledPush> {
    return this.get(`/schedules/${id}`, ProtoScheduledPush);
  }

  async updateSchedule(
    id: number,
    payload: UpdateSchedulePayload,
  ): Promise<ScheduledPush> {
    return this.patch(
      `/schedules/${id}`,
      payload,
      UpdateSchedulePayloadCodec,
      ProtoScheduledPush,
    );
  }

  async deleteSchedule(id: number): Promise<{ message: string }> {
    return this.delete(`/schedules/${id}`, StatusMessage);
  }

  // --- metrics ---------------------------------------------------------

  async getAggregateMetrics(): Promise<PushMetricsAggregate> {
    return this.get("/metrics/aggregate", ProtoPushMetricsAggregate);
  }

  async getCampaignMetrics(id: number): Promise<PushMetricsByCampaign> {
    return this.get(`/metrics/campaign/${id}`, ProtoPushMetricsByCampaign);
  }
}

// The push request types are exported as plain TypeScript
// interfaces from the generated proto; we expose a thin codec
// wrapper so the BaseApiService can encode them with the same
// shape it uses for the rest of the services. Keeping the codecs
// local to this file (instead of mutating the generated proto)
// means a `protoc` regen never has to touch the service layer.

const CreateImmediatePushPayloadCodec: ProtoCodec<CreateImmediatePushPayload> = {
  encode: (m) => CreateImmediatePushRequest.encode(m),
  decode: (b) => CreateImmediatePushRequest.decode(b),
};
const CreateSchedulePayloadCodec: ProtoCodec<CreateSchedulePayload> = {
  encode: (m) => CreateScheduleRequest.encode(m),
  decode: (b) => CreateScheduleRequest.decode(b),
};
const UpdateSchedulePayloadCodec: ProtoCodec<UpdateSchedulePayload> = {
  encode: (m) => UpdateScheduleRequest.encode(m),
  decode: (b) => UpdateScheduleRequest.decode(b),
};

// --- feature flag toggle (sibling endpoint on /users) ----------------

/**
 * FeatureFlagApiService covers the per-user
 * notifications_enabled toggle. It is sibling to the user
 * management surface (`/api/app/v1/users/:id`) and split out
 * here so the push dashboard can wire it without depending on
 * the full UsersApiService surface (which the push page does
 * not otherwise need).
 */
export class FeatureFlagApiService extends BaseApiService {
  constructor() {
    super("/users");
  }

  async setNotificationsEnabled(
    userId: number,
    payload: SetNotificationsEnabledPayload,
  ): Promise<User> {
    return this.put(
      `/${userId}/notifications`,
      payload,
      SetNotificationsEnabledPayloadCodec,
      ProtoUserMessage,
    );
  }
}

const SetNotificationsEnabledPayloadCodec: ProtoCodec<SetNotificationsEnabledPayload> = {
  encode: (m) => SetNotificationsEnabledRequest.encode(m),
  decode: (b) => SetNotificationsEnabledRequest.decode(b),
};

export const pushesApiService = new PushesApiService();
export const featureFlagApiService = new FeatureFlagApiService();
