export interface GormDeletedAt {
  Time?: string;
  Valid?: boolean;
}

export interface GormEntity {
  ID: number;
  CreatedAt: string;
  UpdatedAt: string;
  DeletedAt: GormDeletedAt | null;
}

export interface Team extends GormEntity {
  LogoUrl: string;
}

export interface SofaScoreEvent extends GormEntity {
  SofaScoreEventId: number;
  Sport: string;
  HomeTeam: string;
  HomeScore: number;
  HomeTeamId: number;
  AwayTeam: string;
  AwayScore: number;
  AwayTeamId: number;
  ScrapedAt: number;
  Category: string;
  StartTimestamp: number;
  CurrentPeriodStartTimestamp: number;
  Slug: string;

  teamHome: Team;
  teamAway: Team;
  league: Tournament;
}

export interface EventsResponse {
  events: SofaScoreEvent[];
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

export interface EventsQuery {
  date?: string;
  sport?: string;
  page?: number;
  limit?: number;
}

export interface Device extends GormEntity {
  UserID: number;
  Token: string;
  Platform: string;
  Name: string;
  LastSeen: number;
}

export interface RegisterDevicePayload {
  token: string;
  platform?: string;
  name?: string;
}

export interface PlaybackLog extends GormEntity {
  DeviceID: number;
  SofaScoreEventId: number;
  StartedAt: number;
  EndedAt: number;
}

export interface CreatePlaybackPayload {
  device_token: string;
  sofa_score_event_id: number;
  started_at?: number;
}

export interface UpdatePlaybackPayload {
  ended_at?: number;
}

export interface StatusResponse {
  status: string;
}

export interface EventStats {
  SofaScoreEventId: number;
  ViewCount: number;
}

export interface Tournament extends GormEntity {
  name: string;
  slug: string;
}

export interface DeviceTournament extends GormEntity {
  DeviceID: number;
  TournamentID: number;
  Device?: Device;
  Tournament?: Tournament;
}

export interface GlobalTournamentConfig extends GormEntity {
  TournamentID: number;
  Tournament?: Tournament;
}

export interface CreateTournamentPayload {
  name: string;
  slug: string;
}

export interface UpdateTournamentPayload {
  name: string;
  slug: string;
}

export interface AssignTournamentPayload {
  device_id: number;
  tournament_id: number;
}

export interface SetDeviceTournamentsPayload {
  tournament_ids: number[];
}

export interface SetGlobalConfigPayload {
  tournament_ids: number[];
}

export interface AddGlobalConfigPayload {
  tournament_id: number;
}

export interface UploadApkResponse {
  id: number;
  version: string;
  file_name: string;
  file_size: number;
  description: string;
  package_name: string;
  version_code: number;
  min_sdk_version: number;
  target_sdk_version: number;
  download_token: string;
  download_url: string;
  created_at: string;
}

export interface ApkVersionInfo {
  id: number;
  version: string;
  file_name: string;
  file_size: number;
  description: string;
  is_active: boolean;
  package_name: string;
  version_code: number;
  min_sdk_version: number;
  target_sdk_version: number;
  download_token: string;
  download_url: string;
  created_at: string;
}

export interface ApkCheckResponse {
  update_available: boolean;
  latest_version: string;
  package_name: string;
  version_code: number;
  download_url?: string;
  description?: string;
  file_size?: number;
  min_sdk_version?: number;
  target_sdk_version?: number;
}

export interface ApiErrorResponse {
  error: string;
}

export type PlaybackUpdateMethod = "PUT" | "PATCH";
