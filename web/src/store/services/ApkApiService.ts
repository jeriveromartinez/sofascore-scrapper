import { BaseApiService } from "./BaseApiService";
import {
  ApkList,
  ApkUpdateCheckResponse,
  ApkUploadResponse,
  DeviceUrl,
  StatusMessage,
} from "../../proto/api";
import type {
  ApkCheckResponse,
  ApkVersionInfo,
  UploadApkResponse,
} from "./models";

/** Size of each chunk sent to the server (10 MB – each request stays well below Cloudflare's 50 MB POST limit). */
const CHUNK_SIZE = 10 * 1024 * 1024;

export class ApkApiService extends BaseApiService {
  constructor() {
    super("/apk");
  }

  /**
   * Upload a single chunk of a file as part of a chunked upload session.
   * Uses plain JSON (not CBOR) for maximum throughput.
   * @param uploadId  - UUID identifying the upload session.
   * @param chunkIndex - 0-based index of this chunk.
   * @param totalChunks - Total number of chunks for this upload.
   * @param chunk - The chunk binary data.
   */
  private async uploadChunk(
    uploadId: string,
    chunkIndex: number,
    totalChunks: number,
    chunk: Blob,
  ): Promise<void> {
    const form = new FormData();
    form.append("upload_id", uploadId);
    form.append("chunk_index", String(chunkIndex));
    form.append("total_chunks", String(totalChunks));
    form.append("file", chunk, `chunk-${chunkIndex}`);
    await this.postMultipartJSON<unknown>("/upload/chunk", form);
  }

  /**
   * Assemble all previously uploaded chunks into a final APK version.
   * Uses plain JSON (not CBOR) to match the chunk upload endpoint.
   * @param uploadId    - UUID identifying the upload session.
   * @param totalChunks - Total number of chunks that were uploaded.
   * @param version     - Optional version override (MAJOR.MINOR.PATCH).
   * @param description - Optional release description.
   */
  private async assembleChunks(
    uploadId: string,
    totalChunks: number,
    version?: string,
    description?: string,
  ): Promise<UploadApkResponse> {
    const form = new FormData();
    form.append("upload_id", uploadId);
    form.append("total_chunks", String(totalChunks));
    if (version) form.append("version", version);
    if (description) form.append("description", description);
    return this.postMultipart<UploadApkResponse>(
      "/upload/assemble",
      form,
      ApkUploadResponse,
    );
  }

  /**
   * Upload an APK file.
   * Files larger than CHUNK_SIZE are automatically split into chunks to bypass
   * reverse-proxy body-size limits (e.g. Cloudflare's 100 MB POST limit).
   *
   * @param file        - The APK file to upload.
   * @param version     - Optional version override (MAJOR.MINOR.PATCH).
   * @param description - Optional release description.
   * @param onProgress  - Optional callback receiving upload progress (0–100).
   */
  async uploadApk(
    file: File,
    version?: string,
    description?: string,
    onProgress?: (percent: number) => void,
  ): Promise<UploadApkResponse> {
    if (file.size <= CHUNK_SIZE) {
      // Small file – use the simple single-request upload.
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

    // Large file – split into chunks and upload sequentially.
    const uploadId = crypto.randomUUID();
    const totalChunks = Math.ceil(file.size / CHUNK_SIZE);

    for (let i = 0; i < totalChunks; i++) {
      const start = i * CHUNK_SIZE;
      const end = Math.min(start + CHUNK_SIZE, file.size);
      const chunk = file.slice(start, end);
      await this.uploadChunk(uploadId, i, totalChunks, chunk);
      // Reserve the last 5% of progress for the assemble step.
      onProgress?.(Math.round(((i + 1) / totalChunks) * 95));
    }

    const result = await this.assembleChunks(
      uploadId,
      totalChunks,
      version,
      description,
    );
    onProgress?.(100);
    return result;
  }

  async listVersions(): Promise<ApkVersionInfo[]> {
    return (await this.get("/versions", ApkList)).versions;
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
