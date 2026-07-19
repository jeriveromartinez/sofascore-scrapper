import type {
  ApkInfo as ProtoApkInfo,
  ApkList,
  ApkPage as ProtoApkPage,
  ApkUploadResponse as ProtoApkUploadResponse,
  ApkUpdateCheckResponse as ProtoApkUpdateCheckResponse,
  Domain as ProtoDomain,
  DomainList,
  DomainPage as ProtoDomainPage,
  DomainRequest,
  Device as ProtoDevice,
  DeviceList,
  DevicePage as ProtoDevicePage,
  DeviceRegisterRequest,
  DeviceTournament as ProtoDeviceTournament,
  DeviceTournamentList,
  DeviceTournamentPage as ProtoDeviceTournamentPage,
  EventsList,
  EventPage as ProtoEventPage,
  GlobalTournamentConfig as ProtoGlobalTournamentConfig,
  LogPlaybackRequest,
  PlaybackLog as ProtoPlaybackLog,
  SofaScoreEvent as ProtoSofaScoreEvent,
  Team as ProtoTeam,
  Tournament as ProtoTournament,
  TournamentList,
  TournamentPage as ProtoTournamentPage,
  TournamentRequest,
  SetTournamentIdsRequest,
  AssignTournamentRequest,
  EventStats as ProtoEventStats,
  User as ProtoUser,
  UserList,
  UserPage as ProtoUserPage,
  UserWriteRequest,
} from "../../../proto/api";

export type Team = ProtoTeam;
export type SofaScoreEvent = ProtoSofaScoreEvent;
export type EventsResponse = EventsList;
export type EventPageResponse = ProtoEventPage;
export type DeviceResponse = DeviceList;
export type DevicePageResponse = ProtoDevicePage;
export type UsersResponse = UserList;
export type UserPageResponse = ProtoUserPage;
export type DomainPageResponse = ProtoDomainPage;
export type TournamentPageResponse = ProtoTournamentPage;
export type DeviceTournamentPageResponse = ProtoDeviceTournamentPage;
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
export type ApkPageResponse = ProtoApkPage;

export interface ApiErrorResponse {
  error: string;
}

export type PlaybackUpdateMethod = "PUT" | "PATCH";
