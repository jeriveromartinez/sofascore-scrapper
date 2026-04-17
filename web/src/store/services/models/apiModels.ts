import type {
  ApkInfo as ProtoApkInfo,
  ApkUploadResponse as ProtoApkUploadResponse,
  ApkUpdateCheckResponse as ProtoApkUpdateCheckResponse,
  Domain as ProtoDomain,
  DomainList,
  DomainRequest,
  Device as ProtoDevice,
  DeviceList,
  DeviceRegisterRequest,
  DeviceTournament as ProtoDeviceTournament,
  EventsList,
  GlobalTournamentConfig as ProtoGlobalTournamentConfig,
  LogPlaybackRequest,
  PlaybackLog as ProtoPlaybackLog,
  SofaScoreEvent as ProtoSofaScoreEvent,
  Team as ProtoTeam,
  Tournament as ProtoTournament,
  TournamentRequest,
  SetTournamentIdsRequest,
  AssignTournamentRequest,
  EventStats as ProtoEventStats,
  User as ProtoUser,
  UserList,
  UserWriteRequest,
} from "../../../proto/api";

export type Team = ProtoTeam;
export type SofaScoreEvent = ProtoSofaScoreEvent;
export type EventsResponse = EventsList;
export type DeviceResponse = DeviceList;
export type UsersResponse = UserList;
export type DomainsResponse = DomainList;

export interface EventsQuery {
  date?: string;
  sport?: string;
  page?: number;
  limit?: number;
}

export type Device = ProtoDevice;
export type RegisterDevicePayload = DeviceRegisterRequest;
export type PlaybackLog = ProtoPlaybackLog;
export type CreatePlaybackPayload = LogPlaybackRequest;
export interface UpdatePlaybackPayload {
  endedAt?: number;
}

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
export type SetGlobalConfigPayload = SetTournamentIdsRequest;
export interface AddGlobalConfigPayload {
  tournamentId: number;
}

export type UploadApkResponse = ProtoApkUploadResponse;
export type ApkVersionInfo = ProtoApkInfo;
export type ApkCheckResponse = ProtoApkUpdateCheckResponse;

export interface ApiErrorResponse {
  error: string;
}

export type PlaybackUpdateMethod = "PUT" | "PATCH";
