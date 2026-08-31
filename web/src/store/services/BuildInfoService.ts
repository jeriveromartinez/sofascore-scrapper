// BuildInfoService
//
// REST client for the build-info endpoint. The backend exposes
// GET /api/web/v1/version (public) which returns a BuildInfo
// proto with the version (last semver tag) and commit (short
// SHA) baked into the binary at compile time. See PR #95 in
// the backend repo for the contract.
//
// Wire format is application/x-protobuf, encoded/decoded via
// the generated ts-proto MessageFns wrapper for BuildInfo.

import { BuildInfo } from "../../proto/api";
import { BaseApiService } from "./BaseApiService";

export class BuildInfoService extends BaseApiService {
  constructor() {
    // The endpoint is at the root of the /api/web/v1 group (no
    // sub-path), so super() receives an empty path. get() then
    // appends "/version" to the baseURL.
    super("");
  }

  async getBuildInfo(): Promise<BuildInfo> {
    return this.get("/version", BuildInfo);
  }
}
