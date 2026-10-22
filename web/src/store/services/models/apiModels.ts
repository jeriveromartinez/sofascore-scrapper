import type {
  ApkInfo as ProtoApkInfo,
  ApkPage as ProtoApkPage,
  ApkUploadResponse as ProtoApkUploadResponse,
  ApkUpdateCheckResponse as ProtoApkUpdateCheckResponse,
  Domain as ProtoDomain,
  DomainPage as ProtoDomainPage,
  DomainRequest,
  Device as ProtoDevice,
  DeviceList,
  DevicePage as ProtoDevicePage,
  DeviceRegisterRequest,
  DeviceTournament as ProtoDeviceTournament,
  DeviceTournamentPage as ProtoDeviceTournamentPage,
  EventsList,
  EventPage as ProtoEventPage,
  GlobalTournamentConfig as ProtoGlobalTournamentConfig,
  PlaybackLog as ProtoPlaybackLog,
  PlaybackPage as ProtoPlaybackPage,
  ExternalEvent as ProtoExternalEvent,
  Tournament as ProtoTournament,
  TournamentPage as ProtoTournamentPage,
  TournamentRequest,
  SetTournamentIdsRequest,
  AssignTournamentRequest,
  EventStats as ProtoEventStats,
  User as ProtoUser,
  UserPage as ProtoUserPage,
  UserWriteRequest,
  // Push notification types (added 2026-08-28; see
  // docs/superpowers/specs/2026-08-28-push-notifications-websocket-design.md).
  CreateImmediatePushRequest as ProtoCreateImmediatePushRequest,
  CreateScheduleRequest as ProtoCreateScheduleRequest,
  UpdateScheduleRequest as ProtoUpdateScheduleRequest,
  SetNotificationsEnabledRequest as ProtoSetNotificationsEnabledRequest,
  PushPayload as ProtoPushPayload,
  PushMessage as ProtoPushMessage,
  PushMessagePage as ProtoPushMessagePage,
  ScheduledPush as ProtoScheduledPush,
  ScheduledPushPage as ProtoScheduledPushPage,
  PushMetricsByCampaign as ProtoPushMetricsByCampaign,
  PushMetricsAggregate as ProtoPushMetricsAggregate,
} from "../../../proto/api";

export type ExternalEvent = ProtoExternalEvent;
export type EventsResponse = EventsList;
export type EventPageResponse = ProtoEventPage;
export type DeviceResponse = DeviceList;
export type DevicePageResponse = ProtoDevicePage;
export type UserPageResponse = ProtoUserPage;
export type DomainPageResponse = ProtoDomainPage;
export type TournamentPageResponse = ProtoTournamentPage;
export type DeviceTournamentPageResponse = ProtoDeviceTournamentPage;

export interface EventsQuery {
  date?: string;
  sport?: string;
  page?: number;
  limit?: number;
}

export interface EventsPageFilters {
  direction?: "asc" | "desc";
  from?: string;
  tz?: string;
  sport?: string;
  status?: string;
  league?: string;
  team?: string;
}

export type Device = ProtoDevice;
export type RegisterDevicePayload = DeviceRegisterRequest;
export type PlaybackLog = ProtoPlaybackLog;
export type PlaybackPageResponse = ProtoPlaybackPage;

export interface StatusResponse {
  status?: string;
  message?: string;
}

export type EventStats = ProtoEventStats;
export type User = ProtoUser;
export type Domain = ProtoDomain;
export type Tournament = ProtoTournament;
export type DeviceTournament = ProtoDeviceTournament;
export type GlobalTournamentConfig = ProtoGlobalTournamentConfig;
export type CreateUserPayload = UserWriteRequest;
export type UpdateUserPayload = UserWriteRequest;
export type CreateDomainPayload = DomainRequest;
export type UpdateDomainPayload = DomainRequest;
export type CreateTournamentPayload = TournamentRequest;
export type UpdateTournamentPayload = TournamentRequest;
export type AssignTournamentPayload = AssignTournamentRequest;
export type SetDeviceTournamentsPayload = SetTournamentIdsRequest;

export type UploadApkResponse = ProtoApkUploadResponse;
export type ApkVersionInfo = ProtoApkInfo;
export type ApkCheckResponse = ProtoApkUpdateCheckResponse;
export type ApkPageResponse = ProtoApkPage;

// Push notifications surface (added 2026-08-28).
export type PushPayload = ProtoPushPayload;
export type PushMessage = ProtoPushMessage;
export type PushMessagePageResponse = ProtoPushMessagePage;
export type ScheduledPush = ProtoScheduledPush;
export type ScheduledPushPageResponse = ProtoScheduledPushPage;
export type PushMetricsByCampaign = ProtoPushMetricsByCampaign;
export type PushMetricsAggregate = ProtoPushMetricsAggregate;

export type CreateImmediatePushPayload = ProtoCreateImmediatePushRequest;
export type CreateSchedulePayload = ProtoCreateScheduleRequest;
export type UpdateSchedulePayload = ProtoUpdateScheduleRequest;
export type SetNotificationsEnabledPayload = ProtoSetNotificationsEnabledRequest;

export interface ApiErrorResponse {
  error: string;
}
