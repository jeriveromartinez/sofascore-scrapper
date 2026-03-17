/* eslint-disable */
// Hand-written protobuf encode/decode matching proto/api.proto field numbers.
// Uses protobufjs/minimal as the wire-format library (same as ts-proto output).

import * as _m0 from "protobufjs/minimal";

// ========== Common ==========

export interface ErrorResponse {
  error: string;
}

export const ErrorResponse = {
  encode(
    message: ErrorResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.error !== "") writer.uint32(10).string(message.error);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): ErrorResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { error: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.error = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface StatusMessage {
  message: string;
}

export const StatusMessage = {
  encode(
    message: StatusMessage,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.message !== "") writer.uint32(10).string(message.message);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): StatusMessage {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { message: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.message = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface StatusResponse {
  status: string;
}

export const StatusResponse = {
  encode(
    message: StatusResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.status !== "") writer.uint32(10).string(message.status);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): StatusResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { status: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.status = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Auth / Users ==========

export interface AuthRequest {
  email: string;
  password: string;
}

export const AuthRequest = {
  encode(
    message: AuthRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.email !== "") writer.uint32(10).string(message.email);
    if (message.password !== "") writer.uint32(18).string(message.password);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): AuthRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { email: "", password: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.email = reader.string();
          break;
        case 2:
          message.password = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface AuthResponse {
  id: number;
  email: string;
  token: string;
}

export const AuthResponse = {
  encode(
    message: AuthResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.email !== "") writer.uint32(18).string(message.email);
    if (message.token !== "") writer.uint32(26).string(message.token);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): AuthResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { id: 0, email: "", token: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.email = reader.string();
          break;
        case 3:
          message.token = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Devices ==========

export interface DeviceRegisterRequest {
  token: string;
  platform: string;
  name: string;
}

export const DeviceRegisterRequest = {
  encode(
    message: DeviceRegisterRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.token !== "") writer.uint32(10).string(message.token);
    if (message.platform !== "") writer.uint32(18).string(message.platform);
    if (message.name !== "") writer.uint32(26).string(message.name);
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): DeviceRegisterRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { token: "", platform: "", name: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.token = reader.string();
          break;
        case 2:
          message.platform = reader.string();
          break;
        case 3:
          message.name = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface Device {
  id: number;
  createdAt: string;
  updatedAt: string;
  token: string;
  platform: string;
  name: string;
  lastSeen: number;
}

export const Device = {
  encode(
    message: Device,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.token !== "") writer.uint32(34).string(message.token);
    if (message.platform !== "") writer.uint32(42).string(message.platform);
    if (message.name !== "") writer.uint32(50).string(message.name);
    if (message.lastSeen !== 0) writer.uint32(56).int64(message.lastSeen);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): Device {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      token: "",
      platform: "",
      name: "",
      lastSeen: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.token = reader.string();
          break;
        case 5:
          message.platform = reader.string();
          break;
        case 6:
          message.name = reader.string();
          break;
        case 7:
          message.lastSeen = reader.int64() as unknown as number;
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface DeviceList {
  data: Device[];
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}

export const DeviceList = {
  encode(
    message: DeviceList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.data)
      Device.encode(v, writer.uint32(10).fork()).ldelim();
    if (message.page !== 0) writer.uint32(16).int32(message.page);
    if (message.limit !== 0) writer.uint32(24).int32(message.limit);
    if (message.total !== 0) writer.uint32(32).int64(message.total);
    if (message.totalPages !== 0) writer.uint32(40).int32(message.totalPages);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): DeviceList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: DeviceList = {
      data: [],
      page: 0,
      limit: 0,
      total: 0,
      totalPages: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.data.push(Device.decode(reader, reader.uint32()));
          break;
        case 2:
          message.page = reader.int32();
          break;
        case 3:
          message.limit = reader.int32();
          break;
        case 4:
          message.total = reader.int64() as unknown as number;
          break;
        case 5:
          message.totalPages = reader.int32();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Tournaments ==========

export interface TournamentRequest {
  name: string;
  slug: string;
}

export const TournamentRequest = {
  encode(
    message: TournamentRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") writer.uint32(10).string(message.name);
    if (message.slug !== "") writer.uint32(18).string(message.slug);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): TournamentRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { name: "", slug: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string();
          break;
        case 2:
          message.slug = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface Tournament {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  slug: string;
  region: string;
}

export const Tournament = {
  encode(
    message: Tournament,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.name !== "") writer.uint32(34).string(message.name);
    if (message.slug !== "") writer.uint32(42).string(message.slug);
    if (message.region !== "") writer.uint32(50).string(message.region);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): Tournament {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      name: "",
      slug: "",
      region: "",
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.name = reader.string();
          break;
        case 5:
          message.slug = reader.string();
          break;
        case 6:
          message.region = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface TournamentList {
  tournaments: Tournament[];
}

export const TournamentList = {
  encode(
    message: TournamentList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.tournaments)
      Tournament.encode(v, writer.uint32(10).fork()).ldelim();
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): TournamentList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: TournamentList = { tournaments: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.tournaments.push(Tournament.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Device Tournaments ==========

export interface AssignTournamentRequest {
  deviceId: number;
  tournamentId: number;
}

export const AssignTournamentRequest = {
  encode(
    message: AssignTournamentRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.deviceId !== 0) writer.uint32(8).uint32(message.deviceId);
    if (message.tournamentId !== 0)
      writer.uint32(16).uint32(message.tournamentId);
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): AssignTournamentRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { deviceId: 0, tournamentId: 0 };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.deviceId = reader.uint32();
          break;
        case 2:
          message.tournamentId = reader.uint32();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface SetTournamentIdsRequest {
  tournamentIds: number[];
}

export const SetTournamentIdsRequest = {
  encode(
    message: SetTournamentIdsRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    writer.uint32(10).fork();
    for (const v of message.tournamentIds) writer.uint32(v);
    writer.ldelim();
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): SetTournamentIdsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: SetTournamentIdsRequest = { tournamentIds: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if ((tag & 7) === 2) {
            const end2 = reader.uint32() + reader.pos;
            while (reader.pos < end2)
              message.tournamentIds.push(reader.uint32());
          } else {
            message.tournamentIds.push(reader.uint32());
          }
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface DeviceTournament {
  id: number;
  createdAt: string;
  updatedAt: string;
  deviceId: number;
  tournamentId: number;
  device?: Device;
  tournament?: Tournament;
}

export const DeviceTournament = {
  encode(
    message: DeviceTournament,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.deviceId !== 0) writer.uint32(32).uint32(message.deviceId);
    if (message.tournamentId !== 0)
      writer.uint32(40).uint32(message.tournamentId);
    if (message.device !== undefined)
      Device.encode(message.device, writer.uint32(50).fork()).ldelim();
    if (message.tournament !== undefined)
      Tournament.encode(message.tournament, writer.uint32(58).fork()).ldelim();
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): DeviceTournament {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: DeviceTournament = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      deviceId: 0,
      tournamentId: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.deviceId = reader.uint32();
          break;
        case 5:
          message.tournamentId = reader.uint32();
          break;
        case 6:
          message.device = Device.decode(reader, reader.uint32());
          break;
        case 7:
          message.tournament = Tournament.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface DeviceTournamentList {
  deviceTournaments: DeviceTournament[];
}

export const DeviceTournamentList = {
  encode(
    message: DeviceTournamentList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.deviceTournaments)
      DeviceTournament.encode(v, writer.uint32(10).fork()).ldelim();
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): DeviceTournamentList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: DeviceTournamentList = { deviceTournaments: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.deviceTournaments.push(
            DeviceTournament.decode(reader, reader.uint32()),
          );
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Global Config ==========

export interface GlobalTournamentConfig {
  id: number;
  createdAt: string;
  updatedAt: string;
  tournamentId: number;
  tournament?: Tournament;
}

export const GlobalTournamentConfig = {
  encode(
    message: GlobalTournamentConfig,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.tournamentId !== 0)
      writer.uint32(32).uint32(message.tournamentId);
    if (message.tournament !== undefined)
      Tournament.encode(message.tournament, writer.uint32(42).fork()).ldelim();
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GlobalTournamentConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: GlobalTournamentConfig = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      tournamentId: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.tournamentId = reader.uint32();
          break;
        case 5:
          message.tournament = Tournament.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface GlobalTournamentConfigList {
  configs: GlobalTournamentConfig[];
}

export const GlobalTournamentConfigList = {
  encode(
    message: GlobalTournamentConfigList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.configs)
      GlobalTournamentConfig.encode(v, writer.uint32(10).fork()).ldelim();
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GlobalTournamentConfigList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: GlobalTournamentConfigList = { configs: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.configs.push(
            GlobalTournamentConfig.decode(reader, reader.uint32()),
          );
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Teams ==========

export interface Team {
  id: number;
  teamId: number;
  logoUrl: string;
}

export const Team = {
  encode(message: Team, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.teamId !== 0) writer.uint32(16).int64(message.teamId);
    if (message.logoUrl !== "") writer.uint32(26).string(message.logoUrl);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): Team {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { id: 0, teamId: 0, logoUrl: "" };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.teamId = reader.int64() as unknown as number;
          break;
        case 3:
          message.logoUrl = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Events ==========

export interface SofaScoreEvent {
  id: number;
  createdAt: string;
  updatedAt: string;
  sofaScoreEventId: number;
  sport: string;
  homeTeam: string;
  homeScore: number;
  homeTeamId: number;
  awayTeam: string;
  awayScore: number;
  awayTeamId: number;
  scrapedAt: number;
  category: string;
  startTimestamp: number;
  currentPeriodStartTimestamp: number;
  slug: string;
  teamHome?: Team;
  teamAway?: Team;
  league?: Tournament;
}

export const SofaScoreEvent = {
  encode(
    message: SofaScoreEvent,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.sofaScoreEventId !== 0)
      writer.uint32(32).int64(message.sofaScoreEventId);
    if (message.sport !== "") writer.uint32(42).string(message.sport);
    if (message.homeTeam !== "") writer.uint32(50).string(message.homeTeam);
    if (message.homeScore !== 0) writer.uint32(56).int32(message.homeScore);
    if (message.homeTeamId !== 0) writer.uint32(64).int64(message.homeTeamId);
    if (message.awayTeam !== "") writer.uint32(74).string(message.awayTeam);
    if (message.awayScore !== 0) writer.uint32(80).int32(message.awayScore);
    if (message.awayTeamId !== 0) writer.uint32(88).int64(message.awayTeamId);
    if (message.scrapedAt !== 0) writer.uint32(96).int64(message.scrapedAt);
    if (message.category !== "") writer.uint32(106).string(message.category);
    if (message.startTimestamp !== 0)
      writer.uint32(112).int64(message.startTimestamp);
    if (message.currentPeriodStartTimestamp !== 0)
      writer.uint32(120).int64(message.currentPeriodStartTimestamp);
    if (message.slug !== "") writer.uint32(130).string(message.slug);
    if (message.teamHome !== undefined)
      Team.encode(message.teamHome, writer.uint32(138).fork()).ldelim();
    if (message.teamAway !== undefined)
      Team.encode(message.teamAway, writer.uint32(146).fork()).ldelim();
    if (message.league !== undefined)
      Tournament.encode(message.league, writer.uint32(154).fork()).ldelim();
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): SofaScoreEvent {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: SofaScoreEvent = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      sofaScoreEventId: 0,
      sport: "",
      homeTeam: "",
      homeScore: 0,
      homeTeamId: 0,
      awayTeam: "",
      awayScore: 0,
      awayTeamId: 0,
      scrapedAt: 0,
      category: "",
      startTimestamp: 0,
      currentPeriodStartTimestamp: 0,
      slug: "",
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.sofaScoreEventId = reader.int64() as unknown as number;
          break;
        case 5:
          message.sport = reader.string();
          break;
        case 6:
          message.homeTeam = reader.string();
          break;
        case 7:
          message.homeScore = reader.int32();
          break;
        case 8:
          message.homeTeamId = reader.int64() as unknown as number;
          break;
        case 9:
          message.awayTeam = reader.string();
          break;
        case 10:
          message.awayScore = reader.int32();
          break;
        case 11:
          message.awayTeamId = reader.int64() as unknown as number;
          break;
        case 12:
          message.scrapedAt = reader.int64() as unknown as number;
          break;
        case 13:
          message.category = reader.string();
          break;
        case 14:
          message.startTimestamp = reader.int64() as unknown as number;
          break;
        case 15:
          message.currentPeriodStartTimestamp =
            reader.int64() as unknown as number;
          break;
        case 16:
          message.slug = reader.string();
          break;
        case 17:
          message.teamHome = Team.decode(reader, reader.uint32());
          break;
        case 18:
          message.teamAway = Team.decode(reader, reader.uint32());
          break;
        case 19:
          message.league = Tournament.decode(reader, reader.uint32());
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface EventsList {
  data: SofaScoreEvent[];
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}

export const EventsList = {
  encode(
    message: EventsList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.data)
      SofaScoreEvent.encode(v, writer.uint32(10).fork()).ldelim();
    if (message.page !== 0) writer.uint32(16).int32(message.page);
    if (message.limit !== 0) writer.uint32(24).int32(message.limit);
    if (message.total !== 0) writer.uint32(32).int64(message.total);
    if (message.totalPages !== 0) writer.uint32(40).int32(message.totalPages);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): EventsList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: EventsList = {
      data: [],
      page: 0,
      limit: 0,
      total: 0,
      totalPages: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.data.push(SofaScoreEvent.decode(reader, reader.uint32()));
          break;
        case 2:
          message.page = reader.int32();
          break;
        case 3:
          message.limit = reader.int32();
          break;
        case 4:
          message.total = reader.int64() as unknown as number;
          break;
        case 5:
          message.totalPages = reader.int32();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Playback ==========

export interface LogPlaybackRequest {
  deviceToken: string;
  sofaScoreEventId: number;
  startedAt: number;
}

export const LogPlaybackRequest = {
  encode(
    message: LogPlaybackRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.deviceToken !== "")
      writer.uint32(10).string(message.deviceToken);
    if (message.sofaScoreEventId !== 0)
      writer.uint32(16).int64(message.sofaScoreEventId);
    if (message.startedAt !== 0) writer.uint32(24).int64(message.startedAt);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): LogPlaybackRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { deviceToken: "", sofaScoreEventId: 0, startedAt: 0 };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.deviceToken = reader.string();
          break;
        case 2:
          message.sofaScoreEventId = reader.int64() as unknown as number;
          break;
        case 3:
          message.startedAt = reader.int64() as unknown as number;
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface UpdatePlaybackRequest {
  endedAt: number;
}

export const UpdatePlaybackRequest = {
  encode(
    message: UpdatePlaybackRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.endedAt !== 0) writer.uint32(8).int64(message.endedAt);
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): UpdatePlaybackRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { endedAt: 0 };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.endedAt = reader.int64() as unknown as number;
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface PlaybackLog {
  id: number;
  createdAt: string;
  updatedAt: string;
  deviceId: number;
  sofaScoreEventId: number;
  startedAt: number;
  endedAt: number;
}

export const PlaybackLog = {
  encode(
    message: PlaybackLog,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.createdAt !== "") writer.uint32(18).string(message.createdAt);
    if (message.updatedAt !== "") writer.uint32(26).string(message.updatedAt);
    if (message.deviceId !== 0) writer.uint32(32).uint32(message.deviceId);
    if (message.sofaScoreEventId !== 0)
      writer.uint32(40).int64(message.sofaScoreEventId);
    if (message.startedAt !== 0) writer.uint32(48).int64(message.startedAt);
    if (message.endedAt !== 0) writer.uint32(56).int64(message.endedAt);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): PlaybackLog {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = {
      id: 0,
      createdAt: "",
      updatedAt: "",
      deviceId: 0,
      sofaScoreEventId: 0,
      startedAt: 0,
      endedAt: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.createdAt = reader.string();
          break;
        case 3:
          message.updatedAt = reader.string();
          break;
        case 4:
          message.deviceId = reader.uint32();
          break;
        case 5:
          message.sofaScoreEventId = reader.int64() as unknown as number;
          break;
        case 6:
          message.startedAt = reader.int64() as unknown as number;
          break;
        case 7:
          message.endedAt = reader.int64() as unknown as number;
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== Stats ==========

export interface EventStats {
  sofaScoreEventId: number;
  viewCount: number;
}

export const EventStats = {
  encode(
    message: EventStats,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.sofaScoreEventId !== 0)
      writer.uint32(8).int64(message.sofaScoreEventId);
    if (message.viewCount !== 0) writer.uint32(16).int64(message.viewCount);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): EventStats {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = { sofaScoreEventId: 0, viewCount: 0 };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.sofaScoreEventId = reader.int64() as unknown as number;
          break;
        case 2:
          message.viewCount = reader.int64() as unknown as number;
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface TopEventsResponse {
  stats: EventStats[];
}

export const TopEventsResponse = {
  encode(
    message: TopEventsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.stats)
      EventStats.encode(v, writer.uint32(10).fork()).ldelim();
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): TopEventsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: TopEventsResponse = { stats: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.stats.push(EventStats.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

// ========== APK ==========

export interface ApkInfo {
  id: number;
  version: string;
  fileName: string;
  fileSize: number;
  description: string;
  packageName: string;
  versionCode: number;
  minSdkVersion: number;
  targetSdkVersion: number;
  downloadToken: string;
  downloadUrl: string;
  createdAt: string;
  isActive: boolean;
}

export const ApkInfo = {
  encode(
    message: ApkInfo,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.version !== "") writer.uint32(18).string(message.version);
    if (message.fileName !== "") writer.uint32(26).string(message.fileName);
    if (message.fileSize !== 0) writer.uint32(32).int64(message.fileSize);
    if (message.description !== "")
      writer.uint32(42).string(message.description);
    if (message.packageName !== "")
      writer.uint32(50).string(message.packageName);
    if (message.versionCode !== 0) writer.uint32(56).int32(message.versionCode);
    if (message.minSdkVersion !== 0)
      writer.uint32(64).int32(message.minSdkVersion);
    if (message.targetSdkVersion !== 0)
      writer.uint32(72).int32(message.targetSdkVersion);
    if (message.downloadToken !== "")
      writer.uint32(82).string(message.downloadToken);
    if (message.downloadUrl !== "")
      writer.uint32(90).string(message.downloadUrl);
    if (message.createdAt !== "") writer.uint32(98).string(message.createdAt);
    if (message.isActive !== false) writer.uint32(104).bool(message.isActive);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): ApkInfo {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: ApkInfo = {
      id: 0,
      version: "",
      fileName: "",
      fileSize: 0,
      description: "",
      packageName: "",
      versionCode: 0,
      minSdkVersion: 0,
      targetSdkVersion: 0,
      downloadToken: "",
      downloadUrl: "",
      createdAt: "",
      isActive: false,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.version = reader.string();
          break;
        case 3:
          message.fileName = reader.string();
          break;
        case 4:
          message.fileSize = reader.int64() as unknown as number;
          break;
        case 5:
          message.description = reader.string();
          break;
        case 6:
          message.packageName = reader.string();
          break;
        case 7:
          message.versionCode = reader.int32();
          break;
        case 8:
          message.minSdkVersion = reader.int32();
          break;
        case 9:
          message.targetSdkVersion = reader.int32();
          break;
        case 10:
          message.downloadToken = reader.string();
          break;
        case 11:
          message.downloadUrl = reader.string();
          break;
        case 12:
          message.createdAt = reader.string();
          break;
        case 13:
          message.isActive = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface ApkList {
  versions: ApkInfo[];
}

export const ApkList = {
  encode(
    message: ApkList,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.versions)
      ApkInfo.encode(v, writer.uint32(10).fork()).ldelim();
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): ApkList {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: ApkList = { versions: [] };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.versions.push(ApkInfo.decode(reader, reader.uint32()));
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface ApkUploadResponse {
  id: number;
  version: string;
  fileName: string;
  fileSize: number;
  description: string;
  packageName: string;
  versionCode: number;
  minSdkVersion: number;
  targetSdkVersion: number;
  downloadToken: string;
  downloadUrl: string;
  createdAt: string;
}

export const ApkUploadResponse = {
  encode(
    message: ApkUploadResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== 0) writer.uint32(8).uint32(message.id);
    if (message.version !== "") writer.uint32(18).string(message.version);
    if (message.fileName !== "") writer.uint32(26).string(message.fileName);
    if (message.fileSize !== 0) writer.uint32(32).int64(message.fileSize);
    if (message.description !== "")
      writer.uint32(42).string(message.description);
    if (message.packageName !== "")
      writer.uint32(50).string(message.packageName);
    if (message.versionCode !== 0) writer.uint32(56).int32(message.versionCode);
    if (message.minSdkVersion !== 0)
      writer.uint32(64).int32(message.minSdkVersion);
    if (message.targetSdkVersion !== 0)
      writer.uint32(72).int32(message.targetSdkVersion);
    if (message.downloadToken !== "")
      writer.uint32(82).string(message.downloadToken);
    if (message.downloadUrl !== "")
      writer.uint32(90).string(message.downloadUrl);
    if (message.createdAt !== "") writer.uint32(98).string(message.createdAt);
    return writer;
  },
  decode(input: _m0.Reader | Uint8Array, length?: number): ApkUploadResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: ApkUploadResponse = {
      id: 0,
      version: "",
      fileName: "",
      fileSize: 0,
      description: "",
      packageName: "",
      versionCode: 0,
      minSdkVersion: 0,
      targetSdkVersion: 0,
      downloadToken: "",
      downloadUrl: "",
      createdAt: "",
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.id = reader.uint32();
          break;
        case 2:
          message.version = reader.string();
          break;
        case 3:
          message.fileName = reader.string();
          break;
        case 4:
          message.fileSize = reader.int64() as unknown as number;
          break;
        case 5:
          message.description = reader.string();
          break;
        case 6:
          message.packageName = reader.string();
          break;
        case 7:
          message.versionCode = reader.int32();
          break;
        case 8:
          message.minSdkVersion = reader.int32();
          break;
        case 9:
          message.targetSdkVersion = reader.int32();
          break;
        case 10:
          message.downloadToken = reader.string();
          break;
        case 11:
          message.downloadUrl = reader.string();
          break;
        case 12:
          message.createdAt = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};

export interface ApkUpdateCheckResponse {
  updateAvailable: boolean;
  latestVersion: string;
  packageName: string;
  versionCode: number;
  downloadUrl: string;
  description: string;
  fileSize: number;
  minSdkVersion: number;
  targetSdkVersion: number;
}

export const ApkUpdateCheckResponse = {
  encode(
    message: ApkUpdateCheckResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.updateAvailable !== false)
      writer.uint32(8).bool(message.updateAvailable);
    if (message.latestVersion !== "")
      writer.uint32(18).string(message.latestVersion);
    if (message.packageName !== "")
      writer.uint32(26).string(message.packageName);
    if (message.versionCode !== 0) writer.uint32(32).int32(message.versionCode);
    if (message.downloadUrl !== "")
      writer.uint32(42).string(message.downloadUrl);
    if (message.description !== "")
      writer.uint32(50).string(message.description);
    if (message.fileSize !== 0) writer.uint32(56).int64(message.fileSize);
    if (message.minSdkVersion !== 0)
      writer.uint32(64).int32(message.minSdkVersion);
    if (message.targetSdkVersion !== 0)
      writer.uint32(72).int32(message.targetSdkVersion);
    return writer;
  },
  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ApkUpdateCheckResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message: ApkUpdateCheckResponse = {
      updateAvailable: false,
      latestVersion: "",
      packageName: "",
      versionCode: 0,
      downloadUrl: "",
      description: "",
      fileSize: 0,
      minSdkVersion: 0,
      targetSdkVersion: 0,
    };
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.updateAvailable = reader.bool();
          break;
        case 2:
          message.latestVersion = reader.string();
          break;
        case 3:
          message.packageName = reader.string();
          break;
        case 4:
          message.versionCode = reader.int32();
          break;
        case 5:
          message.downloadUrl = reader.string();
          break;
        case 6:
          message.description = reader.string();
          break;
        case 7:
          message.fileSize = reader.int64() as unknown as number;
          break;
        case 8:
          message.minSdkVersion = reader.int32();
          break;
        case 9:
          message.targetSdkVersion = reader.int32();
          break;
        default:
          reader.skipType(tag & 7);
      }
    }
    return message;
  },
};
