import { BaseApiService } from "./BaseApiService";
import {
  ApkList,
  ApkPage,
  ApkUpdateCheckResponse,
  ApkUploadResponse,
  DeviceUrl,
  StatusMessage,
  UploadBeginRequest,
  UploadBeginResponse,
  UploadStatusResponse,
  UploadChunkResponse,
  UploadCompleteResponse,
} from "../../proto/api";
import type {
  ApkCheckResponse,
  ApkVersionInfo,
  ApkPageResponse,
  UploadApkResponse,
} from "./models";
import { readAuthStorage } from "../authStorage";
import { API_BASE_URL } from "../../constants";
import axios from "axios";

const CHUNK_SIZE = 10 * 1024 * 1024;
const PROTO_CONTENT_TYPE = "application/x-protobuf";
const SESSION_STORAGE_KEY = "apk_upload_session";

interface StoredSession {
  uploadId: string;
  file: { name: string; size: number; type: string };
  totalChunks: number;
  version?: string;
  description?: string;
}

export class ApkApiService extends BaseApiService {
  constructor() {
    super("/apk");
  }

  private saveSession(session: StoredSession): void {
    localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session));
  }

  private loadSession(): StoredSession | null {
    try {
      const raw = localStorage.getItem(SESSION_STORAGE_KEY);
      if (!raw) return null;
      const session = JSON.parse(raw) as StoredSession;
      if (!session.uploadId || !session.file) return null;
      return session;
    } catch {
      return null;
    }
  }

  clearSession(): void {
    localStorage.removeItem(SESSION_STORAGE_KEY);
  }

  async beginUpload(
    fileName: string,
    fileSize: number,
    totalChunks: number,
    version?: string,
    description?: string,
  ): Promise<UploadBeginResponse> {
    return this.post<UploadBeginResponse, UploadBeginRequest>(
      "/uploads",
      {
        file_name: fileName,
        file_size: fileSize,
        total_chunks: totalChunks,
        version: version ?? "",
        description: description ?? "",
      },
      UploadBeginRequest,
      UploadBeginResponse,
    );
  }

  async getUploadStatus(uploadId: string): Promise<UploadStatusResponse> {
    return this.get<UploadStatusResponse>(
      `/uploads/${uploadId}`,
      UploadStatusResponse,
    );
  }

  async putChunk(
    uploadId: string,
    chunkIndex: number,
    chunk: Blob,
  ): Promise<UploadChunkResponse> {
    const token = readAuthStorage().user?.token ?? "";
    const headers: Record<string, string> = {
      Accept: PROTO_CONTENT_TYPE,
      "Content-Type": "application/octet-stream",
    };
    if (token) headers.Authorization = `Bearer ${token}`;

    const arrayBuf = await chunk.arrayBuffer();
    const { data, status } = await axios.put<ArrayBuffer>(
      `${API_BASE_URL}/apk/uploads/${uploadId}/chunks/${chunkIndex}`,
      arrayBuf,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    if (status >= 400) {
      const errBytes = new Uint8Array(data);
      const text = new TextDecoder().decode(errBytes);
      let errMsg = `HTTP ${status}`;
      try {
        const json = JSON.parse(text) as { error?: string };
        if (json.error) errMsg = json.error;
      } catch {
        /* keep default */
      }
      throw new Error(errMsg);
    }

    return UploadChunkResponse.decode(new Uint8Array(data));
  }

  async completeUpload(uploadId: string): Promise<UploadCompleteResponse> {
    return this.postWithoutBody<UploadCompleteResponse>(
      `/uploads/${uploadId}/complete`,
      UploadCompleteResponse,
    );
  }

  async abortUpload(uploadId: string): Promise<void> {
    await this.delete<StatusMessage>(`/uploads/${uploadId}`, StatusMessage);
  }

  async resumeSession(): Promise<{
    session: StoredSession;
    status: UploadStatusResponse;
  } | null> {
    const session = this.loadSession();
    if (!session) return null;

    try {
      const status = await this.getUploadStatus(session.uploadId);
      if (
        status.status === "completed" ||
        status.status === "failed" ||
        status.status === "aborted"
      ) {
        this.clearSession();
        return null;
      }
      return { session, status };
    } catch {
      this.clearSession();
      return null;
    }
  }

  async uploadApk(
    file: File,
    version?: string,
    description?: string,
    onProgress?: (percent: number) => void,
  ): Promise<UploadApkResponse> {
    if (file.size <= CHUNK_SIZE) {
      const form = new FormData();
      form.append("file", file);
      if (version) form.append("version", version);
      if (description) form.append("description", description);
      onProgress?.(100);
      return this.postMultipart<UploadApkResponse>(
        "/upload",
        form,
        ApkUploadResponse,
      );
    }

    const totalChunks = Math.ceil(file.size / CHUNK_SIZE);

    const beginResp = await this.beginUpload(
      file.name,
      file.size,
      totalChunks,
      version,
      description,
    );

    try {
      this.saveSession({
        uploadId: beginResp.upload_id,
        file: { name: file.name, size: file.size, type: file.type },
        totalChunks,
        version,
        description,
      });

      for (let i = 0; i < totalChunks; i++) {
        const start = i * CHUNK_SIZE;
        const end = Math.min(start + CHUNK_SIZE, file.size);
        const chunk = file.slice(start, end);
        await this.putChunk(beginResp.upload_id, i, chunk);
        onProgress?.(Math.round(((i + 1) / totalChunks) * 90));
      }

      const completeResp = await this.completeUpload(beginResp.upload_id);
      this.clearSession();
      onProgress?.(100);

      return {
        id: completeResp.id,
        version: completeResp.version,
        fileName: completeResp.file_name,
        fileSize: completeResp.file_size,
        description: completeResp.description,
        packageName: completeResp.package_name,
        versionCode: completeResp.version_code,
        minSdkVersion: completeResp.min_sdk_version,
        targetSdkVersion: completeResp.target_sdk_version,
        downloadToken: completeResp.download_token,
        downloadUrl: completeResp.download_url,
        createdAt: completeResp.created_at,
      };
    } catch (error) {
      try {
        await this.abortUpload(beginResp.upload_id);
      } catch {
        // best effort abort
      }
      this.clearSession();
      throw error;
    }
  }

  async listVersions(): Promise<ApkVersionInfo[]> {
    return (await this.get("/versions", ApkList)).versions;
  }

  async listVersionsPage(
    cursor?: string,
    limit?: number,
  ): Promise<ApkPageResponse> {
    const params = new URLSearchParams();
    if (cursor) params.set("cursor", cursor);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    const url = qs ? `/versions/page?${qs}` : "/versions/page";
    return this.get(url, ApkPage);
  }

  async checkUpdate(
    version: string,
    packageName: string,
  ): Promise<ApkCheckResponse> {
    return this.get<ApkCheckResponse>(
      `/check?version=${encodeURIComponent(version)}&package=${encodeURIComponent(packageName)}`,
      ApkUpdateCheckResponse,
    );
  }

  getDownloadUrl(appKey: string): string {
    const normalizedPath = appKey.startsWith("/") ? appKey : `/${appKey}`;
    return `${window.location.origin}${normalizedPath}`;
  }

  async downloadByToken(token: string): Promise<Blob> {
    return this.getBinary(`/download/${token}`);
  }

  async updateApkUrl(id: number, url: string): Promise<StatusMessage> {
    return this.put(`/${id}`, { url }, DeviceUrl, StatusMessage);
  }
}

export const apkApiService = new ApkApiService();
