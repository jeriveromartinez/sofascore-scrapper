import { BaseApiService } from "./BaseApiService";
import type {
  ApkCheckResponse,
  ApkVersionInfo,
  UploadApkResponse,
} from "./models";

export class ApkApiService extends BaseApiService {
  constructor() {
    super("/apk");
  }

  async uploadApk(
    file: File,
    version?: string,
    description?: string,
  ): Promise<UploadApkResponse> {
    const form = new FormData();
    form.append("file", file);
    if (version) form.append("version", version);
    if (description) form.append("description", description);

    return this.postMultipart<UploadApkResponse>("/upload", form);
  }

  async listVersions(): Promise<ApkVersionInfo[]> {
    return this.get<ApkVersionInfo[]>("/versions");
  }

  async checkUpdate(version: string): Promise<ApkCheckResponse> {
    return this.get<ApkCheckResponse>(
      `/check?version=${encodeURIComponent(version)}`,
    );
  }

  getDownloadUrl(appKey: string): string {
    const normalizedPath = appKey.startsWith("/") ? appKey : `/${appKey}`;
    return `${window.location.origin}${normalizedPath}`;
  }

  async downloadByToken(token: string): Promise<Blob> {
    return this.getBinary(`/download/${token}`);
  }
}

export const apkApiService = new ApkApiService();
